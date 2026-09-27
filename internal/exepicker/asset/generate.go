//go:build ignore

package main

import (
	"compress/gzip"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	dir, err := os.MkdirTemp("", "typhon-picker-build-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	exe := filepath.Join(dir, "picker.exe")
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=", "-o", exe, "../../../cmd/exepicker")
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err = cmd.Run(); err != nil {
		log.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("picker.exe.gz")
	if err != nil {
		log.Fatal(err)
	}
	z, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		log.Fatal(err)
	}
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
