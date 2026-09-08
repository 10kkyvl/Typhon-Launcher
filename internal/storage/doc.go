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
	"strconv"
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
	// json.RawMessage.UnmarshalJSON always copies its input, so decoding
	// straight into doc{Data json.RawMessage} allocates a second buffer the
	// size of the "data" value on top of the raw bytes already held by the
	// caller: for a 34 MB releases file that is an extra 34 MB alive at the
	// same time as the file bytes, before the result tree is even built.
	// fastSplitEnvelope recognizes the one shape every file this package
	// writes actually has (a clean {"version":N,"data":...} object, in
	// either key order) and returns a slice of trimmed instead of a copy.
	// Anything else - malformed input, a pre-envelope legacy file, unknown
	// extra keys, a duplicate key - falls through to the exact original
	// decode below, unchanged.
	if payload, version, ok := fastSplitEnvelope(trimmed); ok {
		return payload, version, nil
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

// fastSplitEnvelope extracts version and data from trimmed without copying
// the data value, by scanning raw bytes instead of decoding through
// encoding/json. It only recognizes a well-formed top-level object with
// exactly a "version" and a "data" key (any order, no duplicates, version a
// plain non-zero integer); anything else reports ok=false so the caller can
// fall back to the byte-for-byte original behavior.
func fastSplitEnvelope(trimmed []byte) (payload json.RawMessage, version int, ok bool) {
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, 0, false
	}
	i := skipJSONSpace(trimmed, 1)
	if i < len(trimmed) && trimmed[i] == '}' {
		return nil, 0, false
	}
	haveVersion, haveData := false, false
	for {
		if i >= len(trimmed) || trimmed[i] != '"' {
			return nil, 0, false
		}
		keyStart := i
		keyEnd, err := scanJSONString(trimmed, i)
		if err != nil {
			return nil, 0, false
		}
		key := trimmed[keyStart+1 : keyEnd-1]
		i = skipJSONSpace(trimmed, keyEnd)
		if i >= len(trimmed) || trimmed[i] != ':' {
			return nil, 0, false
		}
		i = skipJSONSpace(trimmed, i+1)
		switch string(key) {
		case "version":
			if haveVersion {
				return nil, 0, false
			}
			valEnd, err := scanJSONValue(trimmed, i)
			if err != nil {
				return nil, 0, false
			}
			v, err := strconv.Atoi(string(trimmed[i:valEnd]))
			if err != nil {
				return nil, 0, false
			}
			version, haveVersion = v, true
			i = valEnd
		case "data":
			if haveData {
				return nil, 0, false
			}
			valEnd, err := scanJSONValue(trimmed, i)
			if err != nil {
				return nil, 0, false
			}
			payload, haveData = json.RawMessage(trimmed[i:valEnd]), true
			i = valEnd
		default:
			return nil, 0, false
		}
		i = skipJSONSpace(trimmed, i)
		if i >= len(trimmed) {
			return nil, 0, false
		}
		if trimmed[i] == ',' {
			i = skipJSONSpace(trimmed, i+1)
			continue
		}
		if trimmed[i] == '}' {
			i++
			break
		}
		return nil, 0, false
	}
	if skipJSONSpace(trimmed, i) != len(trimmed) {
		return nil, 0, false
	}
	if !haveVersion || !haveData || version == 0 {
		return nil, 0, false
	}
	// The scan above only balances brackets and toggles string state; it
	// does not enforce full JSON grammar (a value must be followed by ","
	// or the closing bracket, keys must be quoted, no trailing commas...).
	// A document that happens to keep brackets and quotes balanced despite
	// being invalid JSON - e.g. {"a":""data""}, two string literals with no
	// separator - would otherwise be sliced into a "valid-looking" payload
	// instead of being rejected like the original decode rejects it.
	// json.Valid re-scans trimmed to confirm real JSON grammar, not just
	// bracket/quote balance; it reports a bool without allocating or
	// building any value, so it does not reintroduce the copy this function
	// exists to avoid.
	if !json.Valid(trimmed) {
		return nil, 0, false
	}
	return payload, version, true
}

func skipJSONSpace(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func scanJSONString(b []byte, start int) (int, error) {
	i := start + 1
	for i < len(b) {
		switch b[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1, nil
		default:
			i++
		}
	}
	return 0, errors.New("unterminated string")
}

// scanJSONValue returns the offset right after the JSON value starting at
// b[start]. It does not validate the value, only tracks string and bracket
// state well enough to find where it ends; any anomaly is reported as an
// error so fastSplitEnvelope falls back to the exact original decode.
func scanJSONValue(b []byte, start int) (int, error) {
	if start >= len(b) {
		return 0, errors.New("truncated value")
	}
	switch b[start] {
	case '{', '[':
		depth := 0
		inString, escaped := false, false
		for i := start; i < len(b); i++ {
			c := b[i]
			if inString {
				switch {
				case escaped:
					escaped = false
				case c == '\\':
					escaped = true
				case c == '"':
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			}
		}
		return 0, errors.New("truncated value")
	case '"':
		return scanJSONString(b, start)
	default:
		i := start
		for i < len(b) {
			switch b[i] {
			case ',', '}', ']', ' ', '\t', '\n', '\r':
				if i == start {
					return 0, errors.New("empty value")
				}
				return i, nil
			}
			i++
		}
		if i == start {
			return 0, errors.New("empty value")
		}
		return i, nil
	}
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
