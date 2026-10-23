//go:build !windows

package storage

import "os"

func replaceFile(from, to string) error { return os.Rename(from, to) }

func removeFile(path string) error { return os.Remove(path) }

func openRead(path string) (*os.File, error) { return os.Open(path) }
