//go:build !windows && !(darwin && !devmock)

package steamlink

import (
	"context"
	"errors"
	"testing"
)

func TestPublicAPIUnsupported(t *testing.T) {
	if Supported() {
		t.Fatal("Supported() = true without a Steam backend")
	}
	if err := Add(context.Background(), sampleShortcut(), Art{}, Sync{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Add err = %v", err)
	}
	if err := Remove(context.Background(), 1, Sync{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Remove err = %v", err)
	}
	if _, err := Contains(context.Background(), 1); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Contains err = %v", err)
	}
	if _, err := Root(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Root err = %v", err)
	}
}
