package account

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type ProfileLayout struct {
	Version int           `json:"version"`
	Blocks  []LayoutBlock `json:"blocks"`
}

// Config stays raw so a block written by a newer launcher survives an older launcher's save unchanged.
type LayoutBlock struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Width  string          `json:"width"`
	Config json.RawMessage `json:"config,omitempty"`
}

// OptionalLayout separates the three PATCH states: absent keeps the stored layout, null resets it to the default, an object replaces it.
type OptionalLayout struct {
	Set   bool
	Value *ProfileLayout
}

func (o OptionalLayout) IsZero() bool { return !o.Set }

func (o OptionalLayout) MarshalJSON() ([]byte, error) {
	if o.Value == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(o.Value); err != nil {
		return nil, fmt.Errorf("encode profile layout: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (o *OptionalLayout) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*o = OptionalLayout{Set: true}
		return nil
	}
	var layout ProfileLayout
	if err := json.Unmarshal(data, &layout); err != nil {
		return fmt.Errorf("decode profile layout: %w", err)
	}
	*o = OptionalLayout{Set: true, Value: &layout}
	return nil
}

func SetLayout(layout *ProfileLayout) OptionalLayout {
	return OptionalLayout{Set: true, Value: layout}
}
