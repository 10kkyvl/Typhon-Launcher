package delta

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func randomBytes(seed uint64, n int) []byte {
	out := make([]byte, 0, n+sha256.Size)
	var block [16]byte
	binary.LittleEndian.PutUint64(block[:8], seed)
	for counter := uint64(0); len(out) < n; counter++ {
		binary.LittleEndian.PutUint64(block[8:], counter)
		sum := sha256.Sum256(block[:])
		out = append(out, sum[:]...)
	}
	return out[:n]
}

// edited is base with one block rewritten and a few bytes inserted, the shape
// of two builds of the same program.
func edited(base []byte) []byte {
	out := append([]byte(nil), base...)
	mid := len(out) / 2
	copy(out[mid:mid+512], randomBytes(7, 512))
	return append(out[:len(out)/4], append(randomBytes(8, 300), out[len(out)/4:]...)...)
}

func encode(t *testing.T, base, target []byte) []byte {
	t.Helper()
	var patch bytes.Buffer
	if err := Encode(&patch, base, target); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return patch.Bytes()
}

func TestEncodeApplyRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		base []byte
		// target is derived from base.
		target func([]byte) []byte
	}{
		{"small edit", randomBytes(1, 64<<10), edited},
		{"base larger than the minimum window", randomBytes(2, 3<<20), edited},
		{"target is a prefix of the base", randomBytes(3, 128<<10), func(b []byte) []byte { return b[:len(b)/2] }},
		{"target is the base twice", randomBytes(4, 128<<10), func(b []byte) []byte { return append(append([]byte(nil), b...), b...) }},
		{"nothing in common", randomBytes(5, 32<<10), func([]byte) []byte { return randomBytes(6, 32<<10) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.target(tt.base)
			patch := encode(t, tt.base, target)

			var got bytes.Buffer
			n, err := Apply(context.Background(), &got, tt.base, bytes.NewReader(patch), int64(len(target)))
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if n != int64(len(target)) || !bytes.Equal(got.Bytes(), target) {
				t.Fatalf("Apply() rebuilt %d bytes that differ from the target (%d bytes)", n, len(target))
			}
		})
	}
}

func TestPatchSpendsBytesOnlyOnWhatChanged(t *testing.T) {
	base := randomBytes(10, 1<<20)
	target := edited(base)
	patch := encode(t, base, target)
	if limit := len(target) / 20; len(patch) > limit {
		t.Fatalf("patch is %d bytes for a %d byte target with ~800 bytes changed, want at most %d", len(patch), len(target), limit)
	}
}

func TestApplyRefusesWhatIsNotItsPatch(t *testing.T) {
	base := randomBytes(20, 96<<10)
	target := edited(base)
	patch := encode(t, base, target)
	otherBase := randomBytes(21, len(base))

	tests := []struct {
		name  string
		base  []byte
		patch []byte
		want  error
	}{
		{"another base", otherBase, patch, ErrCorrupt},
		{"truncated patch", base, patch[:len(patch)/2], ErrCorrupt},
		{"patch cut right after the header", base, patch[:12], ErrCorrupt},
		{"empty patch", base, nil, ErrCorrupt},
		{"not a patch at all", base, []byte("this is not a zstd frame, just text that is long enough"), ErrCorrupt},
		{"flipped byte inside the patch", base, flip(patch, len(patch)/2), ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			_, err := Apply(context.Background(), &got, tt.base, bytes.NewReader(tt.patch), int64(len(target)))
			if !errors.Is(err, tt.want) {
				t.Fatalf("Apply() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func flip(b []byte, at int) []byte {
	out := append([]byte(nil), b...)
	out[at] ^= 0xff
	return out
}

func TestApplyStopsAtTheOutputLimit(t *testing.T) {
	base := randomBytes(30, 64<<10)
	target := append(randomBytes(31, 64<<10), base...)
	patch := encode(t, base, target)

	var got bytes.Buffer
	n, err := Apply(context.Background(), &got, base, bytes.NewReader(patch), int64(len(target)-1))
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("Apply() error = %v, want ErrOutputTooLarge", err)
	}
	if n >= int64(len(target)) || int64(got.Len()) >= int64(len(target)) {
		t.Fatalf("Apply() wrote %d bytes before it refused, the target has %d", got.Len(), len(target))
	}
}

func TestApplyHonoursACancelledContext(t *testing.T) {
	base := randomBytes(40, 64<<10)
	patch := encode(t, base, edited(base))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var got bytes.Buffer
	_, err := Apply(ctx, &got, base, bytes.NewReader(patch), 1<<20)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Apply() error = %v, want context.Canceled", err)
	}
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestApplyReportsAnOutputThatCannotBeWritten(t *testing.T) {
	base := randomBytes(50, 64<<10)
	target := edited(base)
	patch := encode(t, base, target)
	diskFull := errors.New("disk is full")

	_, err := Apply(context.Background(), failingWriter{err: diskFull}, base, bytes.NewReader(patch), int64(len(target)))
	if !errors.Is(err, diskFull) {
		t.Fatalf("Apply() error = %v, want the writer's error", err)
	}
	if errors.Is(err, ErrCorrupt) {
		t.Fatalf("Apply() blamed the patch for a failing disk: %v", err)
	}
}

func TestApplyAndEncodeRejectUnusableInput(t *testing.T) {
	data := randomBytes(60, 4<<10)
	t.Run("encode with an empty base", func(t *testing.T) {
		if err := Encode(io.Discard, nil, data); !errors.Is(err, ErrEmptyBase) {
			t.Fatalf("Encode() error = %v, want ErrEmptyBase", err)
		}
	})
	t.Run("encode with an empty target", func(t *testing.T) {
		if err := Encode(io.Discard, data, nil); !errors.Is(err, ErrEmptyTarget) {
			t.Fatalf("Encode() error = %v, want ErrEmptyTarget", err)
		}
	})
	t.Run("apply with an empty base", func(t *testing.T) {
		if _, err := Apply(context.Background(), io.Discard, nil, bytes.NewReader(nil), 10); !errors.Is(err, ErrEmptyBase) {
			t.Fatalf("Apply() error = %v, want ErrEmptyBase", err)
		}
	})
	t.Run("apply without a limit", func(t *testing.T) {
		if _, err := Apply(context.Background(), io.Discard, data, bytes.NewReader(nil), 0); !errors.Is(err, ErrBadLimit) {
			t.Fatalf("Apply() error = %v, want ErrBadLimit", err)
		}
	})
	t.Run("apply with a limit past the window", func(t *testing.T) {
		if _, err := Apply(context.Background(), io.Discard, data, bytes.NewReader(nil), maxWindow+1); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("Apply() error = %v, want ErrTooLarge", err)
		}
	})
}

func TestWindowCoversBaseAndTarget(t *testing.T) {
	tests := []struct {
		base, target int
		want         int
		wantErr      error
	}{
		{1, 1, minWindowBits, nil},
		{1 << (minWindowBits - 1), 1 << (minWindowBits - 1), minWindowBits, nil},
		{1<<(minWindowBits-1) + 1, 1 << (minWindowBits - 1), minWindowBits + 1, nil},
		{36 << 20, 36 << 20, 27, nil},
		{maxWindow / 2, maxWindow / 2, maxWindowBits, nil},
		{maxWindow / 2, maxWindow/2 + 1, 0, ErrTooLarge},
		{maxWindow + 1, 1, 0, ErrTooLarge},
	}
	for _, tt := range tests {
		got, err := windowBits(tt.base, tt.target)
		if !errors.Is(err, tt.wantErr) || got != tt.want {
			t.Errorf("windowBits(%d, %d) = %d, %v; want %d, %v", tt.base, tt.target, got, err, tt.want, tt.wantErr)
		}
	}
}
