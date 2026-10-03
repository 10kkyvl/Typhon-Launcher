//go:build ignore

// Run with go generate ./internal/installguard/asset after changing bridge sources.
package main

import (
	"compress/gzip"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"typhon/internal/installguard/asset"
)

func main() {
	sum, err := asset.SourceHash("../../..")
	if err != nil {
		log.Fatal(err)
	}
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
	z.Comment = "source-sha256:" + sum
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
