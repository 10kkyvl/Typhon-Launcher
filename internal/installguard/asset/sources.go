package asset

//go:generate go run generate.go

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var (
	sourceDirs  = []string{"cmd/installguard", "internal/installguard"}
	sourceFiles = []string{"go.mod", "go.sum", "internal/installguard/asset/generate.go"}
)

func SourceHash(root string) (string, error) {
	var names []string
	for _, dir := range sourceDirs {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.Type().IsRegular() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
				names = append(names, dir+"/"+name)
			}
		}
	}
	names = append(names, sourceFiles...)
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return "", err
		}
		// A Windows checkout with autocrlf turns go.mod and go.sum into CRLF files;
		// raw bytes would make the hash depend on the machine that generated it.
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		hash.Write([]byte(name + "\x00" + strconv.Itoa(len(data)) + "\x00"))
		hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
