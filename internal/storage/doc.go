package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

type Migration func(json.RawMessage) (json.RawMessage, error)

type doc struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}

func Load(path string, version int, migrations map[int]Migration, out any) error {
	if path == "" {
		return errors.New("storage path unavailable")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	payload, from, err := split(raw)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if from > version {
		return fmt.Errorf("%s: unsupported version %d", filepath.Base(path), from)
	}
	for from < version {
		migrate := migrations[from]
		if migrate == nil {
			return fmt.Errorf("%s: no migration from version %d", filepath.Base(path), from)
		}
		payload, err = migrate(payload)
		if err != nil {
			return fmt.Errorf("%s: migrate from version %d: %w", filepath.Base(path), from, err)
		}
		from++
	}
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

func split(raw []byte) (json.RawMessage, int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, 0, errors.New("empty document")
	}
	var envelope doc
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		if trimmed[0] != '[' && trimmed[0] != '{' {
			return nil, 0, fmt.Errorf("unexpected document root: %w", err)
		}
		if !json.Valid(trimmed) {
			return nil, 0, fmt.Errorf("malformed document: %w", err)
		}
		return json.RawMessage(trimmed), 0, nil
	}
	if envelope.Version == 0 || envelope.Data == nil {
		return json.RawMessage(trimmed), 0, nil
	}
	return envelope.Data, envelope.Version, nil
}

func Save(path string, version int, data any) error {
	if path == "" {
		return errors.New("storage path unavailable")
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(doc{Version: version, Data: payload}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return WriteAtomic(path, encoded)
}

func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmp := f.Name()
	if err := writeAndSync(f, data, mode); err != nil {
		discardTemp(f)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		discardTemp(f)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		discardTemp(f)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

func writeAndSync(f *os.File, data []byte, mode fs.FileMode) error {
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}
	return f.Sync()
}

func discardTemp(f *os.File) {
	if err := f.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Warn("close temp file", "path", f.Name(), "err", err)
	}
	if err := os.Remove(f.Name()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("remove temp file", "path", f.Name(), "err", err)
	}
}

// Windows не даёт открыть каталог как файл и не поддерживает его fsync, поэтому
// долговечность самой записи каталога здесь best-effort.
func syncDir(dir string) {
	d, err := os.Open(filepath.Clean(dir))
	if err != nil {
		slog.Debug("open dir for sync", "dir", dir, "err", err)
		return
	}
	if err := d.Sync(); err != nil {
		slog.Debug("sync dir", "dir", dir, "err", err)
	}
	if err := d.Close(); err != nil {
		slog.Debug("close dir", "dir", dir, "err", err)
	}
}
