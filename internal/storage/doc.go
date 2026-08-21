package storage

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	payload, from, err := split(raw)
	if err != nil {
		return err
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
	var envelope doc
	if err := json.Unmarshal(raw, &envelope); err != nil {
		var legacy json.RawMessage
		if json.Unmarshal(raw, &legacy) != nil {
			return nil, 0, errors.New("invalid json")
		}
		return legacy, 0, nil
	}
	if envelope.Version == 0 || envelope.Data == nil {
		return json.RawMessage(raw), 0, nil
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
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		slog.Error("write file", "path", tmp, "error", err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		slog.Error("replace file", "path", path, "error", err)
		return err
	}
	return nil
}
