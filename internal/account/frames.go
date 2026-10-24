package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
)

const (
	MinFrameHole = 0.3
	MaxFrameHole = 1.0

	MinFrameCenter = 0.05
	MaxFrameCenter = 0.95
	frameCenterMid = 0.5
)

var (
	ErrFrameURL    = errors.New("avatar frame url is not allowed")
	ErrFrameHole   = errors.New("avatar frame hole is out of range")
	ErrFrameCenter = errors.New("avatar frame hole center is out of range")
)

type FrameImage struct {
	URL   string  `json:"url"`
	Hole  float64 `json:"hole"`
	HoleX float64 `json:"holeX"`
	HoleY float64 `json:"holeY"`
}

type OwnedFrame struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	URL   string  `json:"url"`
	Hole  float64 `json:"hole"`
	HoleX float64 `json:"holeX"`
	HoleY float64 `json:"holeY"`
}

// Older servers omit holeX/holeY; a missing field means a centred hole, while an explicit 0 stays invalid.
func (f *FrameImage) UnmarshalJSON(data []byte) error {
	var raw struct {
		URL   string   `json:"url"`
		Hole  float64  `json:"hole"`
		HoleX *float64 `json:"holeX"`
		HoleY *float64 `json:"holeY"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = FrameImage{URL: raw.URL, Hole: raw.Hole, HoleX: centerOrMid(raw.HoleX), HoleY: centerOrMid(raw.HoleY)}
	return nil
}

func (f *OwnedFrame) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		URL   string   `json:"url"`
		Hole  float64  `json:"hole"`
		HoleX *float64 `json:"holeX"`
		HoleY *float64 `json:"holeY"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*f = OwnedFrame{ID: raw.ID, Name: raw.Name, URL: raw.URL, Hole: raw.Hole, HoleX: centerOrMid(raw.HoleX), HoleY: centerOrMid(raw.HoleY)}
	return nil
}

func centerOrMid(v *float64) float64 {
	if v == nil {
		return frameCenterMid
	}
	return *v
}

func ValidateFrameImage(img FrameImage) error {
	return validateFrame(img.URL, img.Hole, img.HoleX, img.HoleY)
}

func validateFrame(raw string, hole, holeX, holeY float64) error {
	if raw == "" {
		return fmt.Errorf("%w: empty", ErrFrameURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFrameURL, err)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: missing host", ErrFrameURL)
	}
	if err := checkURLScheme(u); err != nil {
		return fmt.Errorf("%w: %w", ErrFrameURL, err)
	}
	if math.IsNaN(hole) || hole < MinFrameHole || hole > MaxFrameHole {
		return fmt.Errorf("%w: %v", ErrFrameHole, hole)
	}
	for _, c := range [...]float64{holeX, holeY} {
		if math.IsNaN(c) || c < MinFrameCenter || c > MaxFrameCenter {
			return fmt.Errorf("%w: %v", ErrFrameCenter, c)
		}
	}
	return nil
}

// SanitizeFrameImage drops an untrusted frame picture that fails validation (invariant 32); the card stays valid, only the frame is lost.
func SanitizeFrameImage(img *FrameImage) *FrameImage {
	if img == nil {
		return nil
	}
	if err := ValidateFrameImage(*img); err != nil {
		slog.Warn("drop avatar frame image", "error", err)
		return nil
	}
	return img
}

func sanitizeFrames(frames []OwnedFrame) []OwnedFrame {
	if frames == nil {
		return nil
	}
	kept := make([]OwnedFrame, 0, len(frames))
	dropped := 0
	var last error
	for _, f := range frames {
		if f.ID == "" {
			dropped++
			last = errors.New("avatar frame has no id")
			continue
		}
		if err := validateFrame(f.URL, f.Hole, f.HoleX, f.HoleY); err != nil {
			dropped++
			last = err
			continue
		}
		kept = append(kept, f)
	}
	if dropped > 0 {
		slog.Warn("drop avatar frames", "dropped", dropped, "error", last)
	}
	return kept
}
