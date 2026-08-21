package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMetainfoForDistinguishesMissingAndBrokenCache(t *testing.T) {
	const infoHash = "0123456789abcdef0123456789abcdef01234567"

	cases := []struct {
		name    string
		prepare func(t *testing.T, dir string)
		want    error
		wantErr bool
	}{
		{
			name:    "no cached file falls through to the source",
			prepare: func(*testing.T, string) {},
			want:    errNoMetadata,
			wantErr: true,
		},
		{
			name: "broken cached file is reported",
			prepare: func(t *testing.T, dir string) {
				path := filepath.Join(dir, "torrents", infoHash+".torrent")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("not a torrent"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.prepare(t, dir)
			m := &Manager{store: newStore(dir)}

			mi, err := m.metainfoFor(context.Background(), nil, "", infoHash)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("metainfoFor: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("metainfoFor = %+v, want error", mi)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.want == nil && errors.Is(err, errNoMetadata) {
				t.Fatalf("err = %v: a broken cache file must not look like a missing one", err)
			}
		})
	}
}
