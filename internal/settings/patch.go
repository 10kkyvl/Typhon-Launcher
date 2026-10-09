package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrPatchUnknownField = errors.New("settings patch: unknown field")
	ErrPatchNullValue    = errors.New("settings patch: null value")
	ErrPatchInvalidValue = errors.New("settings patch: invalid value")
	ErrPatchDerivedField = errors.New("settings patch: field is derived from libraryPath")
)

var derivedFields = map[string]bool{"gamesPath": true, "downloadsPath": true, "screenshotsPath": true}

// SaveSettingsPatch changes only the fields named in patch and leaves the rest
// as stored, so a screen that sends one switch cannot put back an older copy of
// another one saved meanwhile. Keys are the JSON names of Settings. A key that
// is not a field, a path derived from libraryPath (sanitize would overwrite
// it), a null and a value of the wrong type are refused with an
// error that names the key: a field dropped quietly would look saved. An empty
// patch changes nothing and returns the stored settings. The stored settings
// come back so the caller shows what was written rather than what it sent.
func (s *Service) SaveSettingsPatch(patch map[string]json.RawMessage) (Settings, error) {
	if len(patch) == 0 {
		return s.GetSettings(), nil
	}
	return s.Update(func(next *Settings) error {
		patched, err := applyPatch(*next, patch)
		if err != nil {
			return err
		}
		*next = patched
		return nil
	})
}

func applyPatch(current Settings, patch map[string]json.RawMessage) (Settings, error) {
	raw, err := json.Marshal(current)
	if err != nil {
		return Settings{}, fmt.Errorf("encode settings: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Settings{}, fmt.Errorf("decode settings fields: %w", err)
	}
	for key, value := range patch {
		if _, known := fields[key]; !known {
			return Settings{}, fmt.Errorf("%w %q", ErrPatchUnknownField, key)
		}
		if derivedFields[key] {
			return Settings{}, fmt.Errorf("%w: %q", ErrPatchDerivedField, key)
		}
		if !json.Valid(value) {
			return Settings{}, fmt.Errorf("%w for %q: not JSON", ErrPatchInvalidValue, key)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Settings{}, fmt.Errorf("%w for %q", ErrPatchNullValue, key)
		}
		fields[key] = value
	}
	merged, err := json.Marshal(fields)
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %w", ErrPatchInvalidValue, err)
	}
	dec := json.NewDecoder(bytes.NewReader(merged))
	dec.DisallowUnknownFields()
	var out Settings
	if err := dec.Decode(&out); err != nil {
		return Settings{}, fmt.Errorf("%w: %w", ErrPatchInvalidValue, err)
	}
	return out, nil
}
