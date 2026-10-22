package savebackup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Usage is what one game's local save snapshots take, by their manifests. A
// broken snapshot has no readable manifest, so it is counted and the total is
// marked unknown instead of pretending the broken copy is empty.
type Usage struct {
	GameID      string `json:"gameId"`
	Count       int    `json:"count"`
	SizeBytes   int64  `json:"sizeBytes"`
	SizeUnknown bool   `json:"sizeUnknown"`
}

//wails:ignore
func (s *Service) Usage(ctx context.Context) ([]Usage, error) {
	if !s.enabled {
		return nil, errDisabled
	}
	ctx, end, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Stat(s.root); errors.Is(statErr, fs.ErrNotExist) {
			return []Usage{}, nil
		}
		return nil, fmt.Errorf("папка копий %s не является папкой", s.root)
	}
	if err != nil {
		return nil, fmt.Errorf("чтение папки копий: %w", err)
	}
	out := make([]Usage, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !validName(entry.Name()) {
			continue
		}
		u := Usage{GameID: entry.Name()}
		snaps, err := s.list(ctx, u.GameID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			u.SizeUnknown = true
			out = append(out, u)
			continue
		}
		for _, snap := range snaps {
			u.Count++
			u.SizeBytes += snap.SizeBytes
			if snap.Broken {
				u.SizeUnknown = true
			}
		}
		if u.Count > 0 || u.SizeUnknown {
			out = append(out, u)
		}
	}
	return out, nil
}
