package relocate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"typhon/internal/hashdir"
	"typhon/internal/storage"
)

const (
	journalVersion  = 1
	manifestVersion = 1
	journalName     = "moves.json"
	manifestDir     = "moves"
)

type store struct {
	dir string
}

func newStore(dir string) *store {
	return &store{dir: dir}
}

func (st *store) journalPath() string {
	return filepath.Join(st.dir, journalName)
}

func (st *store) manifestPath(jobID string) string {
	return filepath.Join(st.dir, manifestDir, jobID+".json")
}

func (st *store) loadJournal() ([]Job, error) {
	var jobs []Job
	if err := storage.Load(st.journalPath(), journalVersion, nil, &jobs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return jobs, nil
}

func (st *store) saveJournal(jobs []Job) error {
	if err := storage.Save(st.journalPath(), journalVersion, jobs); err != nil {
		return fmt.Errorf("write moves journal: %w", err)
	}
	return nil
}

func (st *store) saveManifest(jobID string, m hashdir.Manifest) error {
	if err := storage.Save(st.manifestPath(jobID), manifestVersion, m); err != nil {
		return fmt.Errorf("write move manifest %s: %w", jobID, err)
	}
	return nil
}

func (st *store) loadManifest(jobID string) (hashdir.Manifest, error) {
	var m hashdir.Manifest
	if err := storage.Load(st.manifestPath(jobID), manifestVersion, nil, &m); err != nil {
		return hashdir.Manifest{}, err
	}
	return m, nil
}

func (st *store) removeManifest(jobID string) error {
	if err := os.Remove(st.manifestPath(jobID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove move manifest %s: %w", jobID, err)
	}
	return nil
}
