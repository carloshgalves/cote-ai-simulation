//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

// sqliteWALProofH2 describes only the validation outcome, never the private
// source page content. SQLite is not allowed to open the detached copy until
// this complete, fail-closed physical WAL scan has succeeded.
type sqliteWALProofH2 struct {
	Frames  int
	Commits int
	HasWAL  bool
	HasSHM  bool
}

func walChecksumH2(data []byte, order binary.ByteOrder, s0, s1 uint32) (uint32, uint32) {
	for i := 0; i < len(data); i += 8 {
		s0 += order.Uint32(data[i:i+4]) + s1
		s1 += order.Uint32(data[i+4:i+8]) + s0
	}
	return s0, s1
}

// verifySQLiteSidecarsH2 rejects journals SQLite could otherwise silently
// ignore, rewind, checkpoint or reconstruct when opening the detached copy.
// It is intentionally stricter than SQLite's crash recovery: an incomplete,
// stale-salt or corrupt frame is never accepted as archived evidence.
// SHM is a volatile index, not durable causal truth; do not hash it as a
// new authority or demand its presence when a well-formed WAL exists.
func verifySQLiteSidecarsH2(original string) (sqliteWALProofH2, error) {
	var proof sqliteWALProofH2
	db, err := readPrivateRegular(original)
	if err != nil {
		return proof, err
	}
	if len(db) < 100 || !bytes.Equal(db[:16], []byte("SQLite format 3\x00")) {
		return proof, errors.New("H2 WAL preflight: invalid SQLite main database header")
	}
	pageSize := int(binary.BigEndian.Uint16(db[16:18]))
	if pageSize == 1 {
		pageSize = 65536
	}
	if pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return proof, errors.New("H2 WAL preflight: invalid main database page size")
	}

	wal, walErr := readPrivateRegular(original + "-wal")
	shm, shmErr := readPrivateRegular(original + "-shm")
	if walErr != nil && !errors.Is(walErr, os.ErrNotExist) {
		return proof, fmt.Errorf("H2 WAL preflight: WAL read: %w", walErr)
	}
	if shmErr != nil && !errors.Is(shmErr, os.ErrNotExist) {
		return proof, fmt.Errorf("H2 WAL preflight: SHM read: %w", shmErr)
	}
	proof.HasWAL = walErr == nil
	proof.HasSHM = shmErr == nil
	if proof.HasSHM {
		// SQLite's wal-index maps 32768-byte shared-memory regions. Partial
		// regions are not completed private evidence; truncate => reject.
		if len(shm) == 0 || len(shm)%32768 != 0 {
			return proof, errors.New("H2 WAL preflight: truncated or malformed transient SHM region")
		}
	}
	if !proof.HasWAL {
		if proof.HasSHM {
			return proof, errors.New("H2 WAL preflight: orphan SHM without corresponding WAL")
		}
		return proof, nil
	}
	if db[18] != 2 || db[19] != 2 {
		return proof, errors.New("H2 WAL preflight: journal conflicts with main SQLite WAL mode")
	}
	if len(wal) < 32 {
		return proof, errors.New("H2 WAL preflight: missing or truncated WAL header")
	}
	magic := binary.BigEndian.Uint32(wal[:4])
	var checksumOrder binary.ByteOrder
	switch magic {
	case 0x377f0682:
		checksumOrder = binary.LittleEndian
	case 0x377f0683:
		checksumOrder = binary.BigEndian
	default:
		return proof, errors.New("H2 WAL preflight: invalid WAL magic")
	}
	if binary.BigEndian.Uint32(wal[4:8]) != 3007000 ||
		binary.BigEndian.Uint32(wal[8:12]) != uint32(pageSize) {
		return proof, errors.New("H2 WAL preflight: version/page size differs from main database")
	}
	s0, s1 := walChecksumH2(wal[:24], checksumOrder, 0, 0)
	if binary.BigEndian.Uint32(wal[24:28]) != s0 ||
		binary.BigEndian.Uint32(wal[28:32]) != s1 {
		return proof, errors.New("H2 WAL preflight: WAL header checksum invalid")
	}
	frameSize := 24 + pageSize
	if len(wal) == 32 || (len(wal)-32)%frameSize != 0 {
		return proof, errors.New("H2 WAL preflight: incomplete or empty WAL frame sequence")
	}
	salt1 := binary.BigEndian.Uint32(wal[16:20])
	salt2 := binary.BigEndian.Uint32(wal[20:24])
	for off := 32; off < len(wal); off += frameSize {
		frame := wal[off : off+frameSize]
		if binary.BigEndian.Uint32(frame[:4]) == 0 ||
			binary.BigEndian.Uint32(frame[8:12]) != salt1 ||
			binary.BigEndian.Uint32(frame[12:16]) != salt2 {
			return proof, errors.New("H2 WAL preflight: invalid page number or stale WAL frame salt")
		}
		s0, s1 = walChecksumH2(frame[:8], checksumOrder, s0, s1)
		s0, s1 = walChecksumH2(frame[24:], checksumOrder, s0, s1)
		if binary.BigEndian.Uint32(frame[16:20]) != s0 ||
			binary.BigEndian.Uint32(frame[20:24]) != s1 {
			return proof, errors.New("H2 WAL preflight: corrupted or partially written WAL frame checksum")
		}
		proof.Frames++
		if binary.BigEndian.Uint32(frame[4:8]) != 0 {
			proof.Commits++
		}
	}
	if proof.Commits == 0 {
		return proof, errors.New("H2 WAL preflight: no complete committed WAL transaction")
	}
	return proof, nil
}
