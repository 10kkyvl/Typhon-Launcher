package screenshots

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Usage is what one game's screenshot folder takes. SizeUnknown is set when
// the folder could not be read completely: the numbers are then a lower bound.
type Usage struct {
	GameID      string `json:"gameId"`
	Count       int    `json:"count"`
	SizeBytes   int64  `json:"sizeBytes"`
	SizeUnknown bool   `json:"sizeUnknown"`
}

// Usage walks the screenshot folder once and reports every game folder in it,
// whether or not the game is still in the library.
//
//wails:ignore
func (s *Service) Usage(ctx context.Context) ([]Usage, error) {
	root, err := s.root()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Stat(root); errors.Is(statErr, fs.ErrNotExist) {
			return []Usage{}, nil
		}
		return nil, fmt.Errorf("read screenshots folder: %s is not a folder", root)
	}
	if err != nil {
		return nil, fmt.Errorf("read screenshots folder: %w", err)
	}
	out := make([]Usage, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !validSegment(entry.Name()) {
			continue
		}
		u := Usage{GameID: entry.Name()}
		shots, err := s.List(entry.Name())
		if err != nil {
			u.SizeUnknown = true
			out = append(out, u)
			continue
		}
		if len(shots) == 0 {
			continue
		}
		for _, shot := range shots {
			u.Count++
			u.SizeBytes += shot.Size
		}
		out = append(out, u)
	}
	return out, nil
}
