//go:build ignore

// Run with go generate ./internal/installguard/asset after changing bridge sources.
package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	dir, err := os.MkdirTemp("", "typhon-guard-build-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	exe := filepath.Join(dir, "helper.exe")
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=", "-o", exe, "../../../cmd/installguard")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("helper.exe.gz")
	if err != nil {
		log.Fatal(err)
	}
	z, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
	hash := sha256.New()
	for _, path := range []string{"../../../cmd/installguard/main_windows.go", "../guard_windows.go", "../job_windows.go", "../policy.go", "../options_windows.go", "../checklist_wine_windows.go", "../../../go.mod", "../../../go.sum", "generate.go"} {
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			log.Fatal(readErr)
		}
		hash.Write(source)
	}
	z.Comment = "source-sha256:" + hex.EncodeToString(hash.Sum(nil))
	if _, err = z.Write(data); err != nil {
		log.Fatal(err)
	}
	if err = z.Close(); err != nil {
		log.Fatal(err)
	}
	if err = f.Close(); err != nil {
		log.Fatal(err)
	}
}
