package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var gameFiles = []tFile{{"game.exe", 40000}, {"d1.dat", 20000}, {"d2.dat", 20000}, {"d3.dat", 20000}, {"d4.dat", 20000}}

func reuseManager(t *testing.T, files []tFile) (m *Manager, hash, parent string) {
	t.Helper()
	mi, parent := buildTorrent(t, "Game", files)
	hash = mi.HashInfoBytes().HexString()
	m = managerWithClient(t, 1)
	if err := m.store.saveMetainfo(hash, mi); err != nil {
		t.Fatal(err)
	}
	return m, hash, parent
}

func TestInspectReuseReportsWhatCanBeReused(t *testing.T) {
	const total = 120000
	cases := []struct {
		name   string
		path   func(parent string) string
		damage func(t *testing.T, parent string)
		force  *bool
		check  func(t *testing.T, r ReuseReport)
	}{
		{"intact data below the torrent folder", func(p string) string { return p }, nil, nil, func(t *testing.T, r ReuseReport) {
			if !r.Applicable || r.Flat || r.PresentFiles != 5 || r.TotalBytes != total || r.TotalPieces != 8 {
				t.Fatalf("report = %+v", r)
			}
			if r.MissingFiles != 0 || r.Name != "Game" || r.Layout != LayoutDirectFiles || len(r.Files) != 5 {
				t.Fatalf("report = %+v", r)
			}
			for _, f := range r.Files {
				if f.Missing {
					t.Fatalf("file = %+v", f)
				}
			}
		}},
		{"intact data directly in the chosen folder", func(p string) string { return filepath.Join(p, "Game") }, nil, nil, func(t *testing.T, r ReuseReport) {
			if !r.Applicable || !r.Flat || r.PresentFiles != 5 || r.Files[0].Path != "d1.dat" || r.Files[4].Path != "game.exe" {
				t.Fatalf("report = %+v", r)
			}
		}},
		{"one damaged piece", func(p string) string { return p }, func(t *testing.T, p string) {
			flipByte(t, filepath.Join(p, "Game"), "d2.dat", 100)
		}, nil, func(t *testing.T, r ReuseReport) {
			if !r.Applicable || r.BadPieces == 0 || r.OkPieces+r.BadPieces != r.TotalPieces {
				t.Fatalf("pieces ok/bad/total = %d/%d/%d", r.OkPieces, r.BadPieces, r.TotalPieces)
			}
			if r.MatchedBytes >= total || r.MissingBytes != total-r.MatchedBytes || r.MissingFiles != 0 {
				t.Fatalf("report = %+v", r)
			}
		}},
		{"a file that is gone", func(p string) string { return p }, func(t *testing.T, p string) {
			if err := os.Remove(filepath.Join(p, "Game", "d3.dat")); err != nil {
				t.Fatal(err)
			}
		}, nil, func(t *testing.T, r ReuseReport) {
			if !r.Applicable || r.MissingFiles != 1 || r.PresentFiles != 4 {
				t.Fatalf("report = %+v", r)
			}
			missing := 0
			for _, f := range r.Files {
				if f.Missing {
					missing++
					if f.Path != filepath.Join("Game", "d3.dat") || f.BytesDone != 0 {
						t.Fatalf("missing file = %+v", f)
					}
				}
			}
			if missing != 1 || r.MatchedBytes >= total {
				t.Fatalf("report = %+v", r)
			}
		}},
		{"a folder the torrent does not describe", func(string) string { return t.TempDir() }, nil, nil, func(t *testing.T, r ReuseReport) {
			if r.Applicable || r.PresentFiles != 0 || r.MissingFiles != 5 || r.MissingBytes != total || r.MatchedBytes != 0 || len(r.Files) != 5 {
				t.Fatalf("report = %+v", r)
			}
			for _, f := range r.Files {
				if !f.Missing {
					t.Fatalf("file = %+v", f)
				}
			}
		}},
		{"a layout forced against the data", func(p string) string { return p }, nil, boolPtr(true), func(t *testing.T, r ReuseReport) {
			if r.Applicable || !r.Flat {
				t.Fatalf("report = %+v: flat was demanded and there is nothing flat on disk", r)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, hash, parent := reuseManager(t, gameFiles)
			if c.damage != nil {
				c.damage(t, parent)
			}

			report, err := m.InspectReuse(t.Context(), ReuseRequest{InfoHash: hash, Path: c.path(parent), Flat: c.force}, nil)
			if err != nil {
				t.Fatalf("InspectReuse: %v", err)
			}

			if report.InfoHash != hash {
				t.Fatalf("hash = %s", report.InfoHash)
			}
			c.check(t, report)
			if report.MatchedBytes+report.MissingBytes != report.TotalBytes || report.OkPieces+report.BadPieces != report.TotalPieces && report.Applicable {
				t.Fatalf("the numbers of the report do not add up: %+v", report)
			}
			if got := len(m.client.cl.Torrents()); got != 0 {
				t.Fatalf("a probe torrent was left in the client: %d", got)
			}
			if _, reserved := m.bookkeeping(); reserved != 0 {
				t.Fatalf("reserved = %d: the hash is stuck busy", reserved)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

func TestInspectReuseReportsProgressUpToTheEnd(t *testing.T) {
	m, hash, parent := reuseManager(t, gameFiles)
	var seen []VerifyProgress

	_, err := m.InspectReuse(t.Context(), ReuseRequest{InfoHash: hash, Path: parent}, func(p VerifyProgress) {
		seen = append(seen, p)
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(seen) == 0 {
		t.Fatal("no progress was reported")
	}
	var last int64
	for _, p := range seen {
		if p.ProcessedBytes <= last || p.TotalBytes != 120000 || p.CurrentFile == "" {
			t.Fatalf("progress = %+v after %d", p, last)
		}
		last = p.ProcessedBytes
	}
	if last != 120000 {
		t.Fatalf("progress ended at %d, want the whole torrent", last)
	}
	if first := seen[0].CurrentFile; first != filepath.Join("Game", "d1.dat") {
		t.Fatalf("first file reported = %q", first)
	}
}

func TestInspectReuseRefusals(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		setup func(t *testing.T, m *Manager, hash, parent string) (ReuseRequest, context.Context)
		want  error
	}{
		{"blank path", func(t *testing.T, m *Manager, hash, parent string) (ReuseRequest, context.Context) {
			return ReuseRequest{InfoHash: hash, Path: "  "}, t.Context()
		}, nil},
		{"path is a file", func(t *testing.T, m *Manager, hash, parent string) (ReuseRequest, context.Context) {
			return ReuseRequest{InfoHash: hash, Path: file}, t.Context()
		}, nil},
		{"path does not exist", func(t *testing.T, m *Manager, hash, parent string) (ReuseRequest, context.Context) {
			return ReuseRequest{InfoHash: hash, Path: filepath.Join(parent, "nowhere")}, t.Context()
		}, nil},
		{"nothing known about the torrent", func(t *testing.T, m *Manager, hash, parent string) (ReuseRequest, context.Context) {
			return ReuseRequest{InfoHash: "a748597437835a2fd0d2e06f8edd86fee316a84d", Path: parent}, t.Context()
		}, errNoMetadata},
		{"another operation holds the hash", func(t *testing.T, m *Manager, hash, parent string) (ReuseRequest, context.Context) {
			m.mu.Lock()
			m.reserved[hash] = true
			m.mu.Unlock()
			return ReuseRequest{InfoHash: hash, Path: parent}, t.Context()
		}, errHashBusy},
		{"the caller gives up", func(t *testing.T, m *Manager, hash, parent string) (ReuseRequest, context.Context) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ReuseRequest{InfoHash: hash, Path: parent}, ctx
		}, context.Canceled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, hash, parent := reuseManager(t, gameFiles)
			req, ctx := c.setup(t, m, hash, parent)
			_, reservedBefore := m.bookkeeping()

			_, err := m.InspectReuse(ctx, req, nil)

			if err == nil {
				t.Fatal("the request was accepted")
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
			if got := len(m.client.cl.Torrents()); got != 0 {
				t.Fatalf("a refused inspection left %d torrents in the client", got)
			}
			if _, reserved := m.bookkeeping(); reserved != reservedBefore {
				t.Fatalf("reserved = %d, was %d: a refusal must give the hash back", reserved, reservedBefore)
			}
		})
	}
}

func TestInspectReuseStopsWhenTheCallerCancelsMidway(t *testing.T) {
	m, hash, parent := reuseManager(t, gameFiles)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_, err := m.InspectReuse(ctx, ReuseRequest{InfoHash: hash, Path: parent}, func(VerifyProgress) { cancel() })

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := len(m.client.cl.Torrents()); got != 0 {
		t.Fatalf("torrents left in the client: %d", got)
	}
	if _, reserved := m.bookkeeping(); reserved != 0 {
		t.Fatalf("reserved = %d", reserved)
	}
}

func TestInspectReuseWithoutAClient(t *testing.T) {
	m := newTestManager(t, 1)
	if _, err := m.InspectReuse(t.Context(), ReuseRequest{Path: t.TempDir()}, nil); !errors.Is(err, errNoClient) {
		t.Fatalf("error = %v, want errNoClient", err)
	}
}
