package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCausalCoreHasNoModelRandomUUIDOrWallClockAuthority(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "csf")
	forbidden := []string{
		`"math/rand"`, `"github.com/google/uuid"`, `"time"`,
		"openai", "anthropic", "langchain", "vector database",
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, token := range forbidden {
			if strings.Contains(strings.ToLower(string(content)), strings.ToLower(token)) {
				t.Errorf("%s contains forbidden causal dependency %q", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSHA256AuthorityIsConfinedToRegisteredAdapters(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "csf")
	allowed := map[string]bool{"identity": true, "policy": true, "conformance": true}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(content), `"crypto/sha256"`) {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		owner := strings.Split(filepath.ToSlash(relative), "/")[0]
		if !allowed[owner] {
			t.Errorf("%s can create an unregistered generic causal hash", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
