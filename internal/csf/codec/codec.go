// Package codec is the only adapter allowed to depend on the CBOR library.
package codec

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"unicode/utf8"

	"github.com/carloshgalves/cote-ai-simulation/internal/csf/domain"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrNonCanonical = errors.New("non-canonical CBOR")
	ErrNonFinite    = errors.New("non-finite float")
	magic           = []byte{'C', 'S', 'F', 0}
	encMode         cbor.EncMode
	decMode         cbor.DecMode
)

func init() {
	var err error
	encMode, err = cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	decMode, err = (cbor.DecOptions{
		DupMapKey:         cbor.DupMapKeyEnforcedAPF,
		IndefLength:       cbor.IndefLengthForbidden,
		TagsMd:            cbor.TagsForbidden,
		UTF8:              cbor.UTF8RejectInvalid,
		MaxNestedLevels:   64,
		MaxArrayElements:  65535,
		MaxMapPairs:       65535,
		ExtraReturnErrors: cbor.ExtraDecErrorUnknownField,
	}).DecMode()
	if err != nil {
		panic(err)
	}
}

func CanonicalBytes(domainTag, schemaID string, schemaVersion uint32, value any) ([]byte, error) {
	if err := domain.ValidateDomainTag(domainTag); err != nil {
		return nil, err
	}
	if err := domain.ValidateDomainTag(schemaID); err != nil {
		return nil, err
	}
	normalized, err := normalize(value)
	if err != nil {
		return nil, err
	}
	encoded, err := encMode.Marshal([]any{magic, uint64(1), domainTag, schemaID, uint64(schemaVersion), normalized})
	if err != nil {
		return nil, fmt.Errorf("encode canonical envelope: %w", err)
	}
	if len(encoded) > 16_777_216 {
		return nil, fmt.Errorf("canonical envelope exceeds size limit")
	}
	return encoded, nil
}

func CanonicalPayload(value any) ([]byte, error) {
	normalized, err := normalize(value)
	if err != nil {
		return nil, err
	}
	return encMode.Marshal(normalized)
}

func StrictDecodePayload(encoded []byte) (any, error) {
	var value any
	if err := decMode.Unmarshal(encoded, &value); err != nil {
		return nil, fmt.Errorf("decode strict CBOR: %w", err)
	}
	reencoded, err := CanonicalPayload(value)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(encoded, reencoded) {
		return nil, ErrNonCanonical
	}
	return value, nil
}

type Envelope struct {
	DomainTag     string
	SchemaID      string
	SchemaVersion uint32
	Value         any
}

func StrictDecodeEnvelope(encoded []byte) (Envelope, error) {
	value, err := StrictDecodePayload(encoded)
	if err != nil {
		return Envelope{}, err
	}
	items, ok := value.([]any)
	if !ok || len(items) != 6 {
		return Envelope{}, fmt.Errorf("envelope must contain exactly six items")
	}
	gotMagic, ok := items[0].([]byte)
	if !ok || !bytes.Equal(gotMagic, magic) {
		return Envelope{}, fmt.Errorf("invalid envelope magic")
	}
	version, ok := unsigned(items[1])
	if !ok || version != 1 {
		return Envelope{}, fmt.Errorf("unsupported codec version")
	}
	domainTag, ok := items[2].(string)
	if !ok || domain.ValidateDomainTag(domainTag) != nil {
		return Envelope{}, fmt.Errorf("invalid envelope domain tag")
	}
	schemaID, ok := items[3].(string)
	if !ok || domain.ValidateDomainTag(schemaID) != nil {
		return Envelope{}, fmt.Errorf("invalid envelope schema id")
	}
	schemaVersion, ok := unsigned(items[4])
	if !ok || schemaVersion > math.MaxUint32 {
		return Envelope{}, fmt.Errorf("invalid schema version")
	}
	return Envelope{DomainTag: domainTag, SchemaID: schemaID, SchemaVersion: uint32(schemaVersion), Value: items[5]}, nil
}

func unsigned(value any) (uint64, bool) {
	switch v := value.(type) {
	case uint64:
		return v, true
	case uint32:
		return uint64(v), true
	case uint16:
		return uint64(v), true
	case uint8:
		return uint64(v), true
	case uint:
		return uint64(v), true
	default:
		return 0, false
	}
}

func normalize(value any) (any, error) {
	switch v := value.(type) {
	case nil, bool, []byte:
		return v, nil
	case string:
		if !utf8.ValidString(v) {
			return nil, fmt.Errorf("invalid UTF-8")
		}
		if containsUnicode15Unassigned(v) {
			return nil, fmt.Errorf("Unicode 15.1 unassigned code point")
		}
		normalized := norm.NFC.String(v)
		if containsUnicode15Unassigned(normalized) {
			return nil, fmt.Errorf("Unicode 15.1 unassigned code point after normalization")
		}
		if len([]byte(normalized)) > 1_048_576 {
			return nil, fmt.Errorf("text exceeds byte limit")
		}
		return normalized, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, ErrNonFinite
		}
		if v == 0 {
			return float64(0), nil
		}
		return v, nil
	case float32:
		return normalize(float64(v))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return v, nil
	case []any:
		result := make([]any, len(v))
		for index, item := range v {
			normalized, err := normalize(item)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	case map[any]any:
		result := make(map[any]any, len(v))
		for key, item := range v {
			normalizedKey, err := normalize(key)
			if err != nil {
				return nil, err
			}
			if reflect.TypeOf(normalizedKey) != nil && !reflect.TypeOf(normalizedKey).Comparable() {
				return nil, fmt.Errorf("complex map key is forbidden")
			}
			if _, duplicate := result[normalizedKey]; duplicate {
				return nil, fmt.Errorf("duplicate map key after normalization")
			}
			normalizedItem, err := normalize(item)
			if err != nil {
				return nil, err
			}
			result[normalizedKey] = normalizedItem
		}
		return result, nil
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() && rv.Kind() == reflect.Slice {
			result := make([]any, rv.Len())
			for index := range result {
				normalized, err := normalize(rv.Index(index).Interface())
				if err != nil {
					return nil, err
				}
				result[index] = normalized
			}
			return result, nil
		}
		return value, nil
	}
}

func containsUnicode15Unassigned(value string) bool {
	for _, codePoint := range value {
		low, high := 0, len(unicode15Unassigned)
		for low < high {
			middle := low + (high-low)/2
			entry := unicode15Unassigned[middle]
			if codePoint < entry[0] {
				high = middle
			} else if codePoint > entry[1] {
				low = middle + 1
			} else {
				return true
			}
		}
	}
	return false
}
