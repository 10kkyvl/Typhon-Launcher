package account

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
)

const (
	MinFrameHole = 0.3
	MaxFrameHole = 1.0
)

var (
	ErrFrameURL  = errors.New("avatar frame url is not allowed")
	ErrFrameHole = errors.New("avatar frame hole is out of range")
)

type FrameImage struct {
	URL  string  `json:"url"`
	Hole float64 `json:"hole"`
}

type OwnedFrame struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	URL  string  `json:"url"`
	Hole float64 `json:"hole"`
}

func ValidateFrameImage(img FrameImage) error {
	return validateFrame(img.URL, img.Hole)
}

func validateFrame(raw string, hole float64) error {
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
		if err := validateFrame(f.URL, f.Hole); err != nil {
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
