package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"sync"

	"typhon/internal/storage"
	"typhon/internal/version"
)

const (
	notesName    = "release-notes.json"
	notesVersion = 1
)

type notesState struct {
	Releases        []ReleaseNote `json:"releases,omitempty"`
	LastSeenVersion string        `json:"lastSeenVersion,omitempty"`
}

type notesStore struct {
	mu       sync.Mutex
	path     string
	readOnly bool
}

func newNotesStore(configDir string) (*notesStore, error) {
	dir, err := CacheDir(configDir)
	if err != nil {
		return nil, err
	}
	return &notesStore{path: filepath.Join(dir, notesName)}, nil
}

func (s *notesStore) Load() (notesState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var v notesState
	err := storage.Load(s.path, notesVersion, nil, &v)
	if errors.Is(err, fs.ErrNotExist) {
		return notesState{}, nil
	}
	if err != nil {
		s.readOnly = true
		return notesState{}, fmt.Errorf("load release notes: %w", err)
	}
	if err := validateReleaseNotes(v.Releases); err != nil {
		s.readOnly = true
		return notesState{}, fmt.Errorf("load release notes: %w", err)
	}
	return v, nil
}

func (s *notesStore) Save(v notesState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return ErrReadOnly
	}
	if err := storage.Save(s.path, notesVersion, v); err != nil {
		return fmt.Errorf("save release notes: %w", err)
	}
	return nil
}

type parsedNote struct {
	note    ReleaseNote
	version version.Version
}

// mergeReleaseNotes keeps entries the server has already trimmed away: a user
// who skips several releases still gets the history of every version between
// the one they left and the one they landed on.
func mergeReleaseNotes(existing, incoming []ReleaseNote) ([]ReleaseNote, error) {
	parsed := make([]parsedNote, 0, len(existing)+len(incoming))
	index := make(map[string]int, len(existing)+len(incoming))

	add := func(n ReleaseNote) error {
		if err := n.Validate(); err != nil {
			return err
		}
		v := version.Parse(n.Version)
		if i, ok := index[v.Normalized]; ok {
			parsed[i] = parsedNote{note: n, version: v}
			return nil
		}
		index[v.Normalized] = len(parsed)
		parsed = append(parsed, parsedNote{note: n, version: v})
		return nil
	}

	for _, n := range existing {
		if err := add(n); err != nil {
			return nil, err
		}
	}
	for _, n := range incoming {
		if err := add(n); err != nil {
			return nil, err
		}
	}

	var sortErr error
	sort.SliceStable(parsed, func(i, j int) bool {
		newer, ok := version.Newer(parsed[i].version, parsed[j].version)
		if !ok {
			sortErr = fmt.Errorf("%w: %q and %q", ErrInvalidReleaseNote, parsed[i].note.Version, parsed[j].note.Version)
			return false
		}
		return newer
	})
	if sortErr != nil {
		return nil, sortErr
	}

	if len(parsed) > MaxReleaseNotes {
		parsed = parsed[:MaxReleaseNotes]
	}
	merged := make([]ReleaseNote, len(parsed))
	for i, p := range parsed {
		merged[i] = p.note
	}
	return merged, nil
}

func unseenReleaseNotes(history []ReleaseNote, lastSeen, current string) ([]ReleaseNote, error) {
	if lastSeen == "" || current == "" {
		return nil, nil
	}
	var unseen []ReleaseNote
	for _, n := range history {
		afterSeen, err := IsNewer(n.Version, lastSeen)
		if err != nil {
			return nil, err
		}
		if !afterSeen {
			continue
		}
		aheadOfCurrent, err := IsNewer(n.Version, current)
		if err != nil {
			return nil, err
		}
		if aheadOfCurrent {
			continue
		}
		unseen = append(unseen, n)
	}
	return unseen, nil
}
