package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"typhon/internal/storage"
)

type updateSpec struct {
	InstallerPath string `json:"installerPath"`
	ParentPID     int    `json:"parentPid"`
	RelaunchPath  string `json:"relaunchPath"`
}

var (
	parentExitTimeout  = 30 * time.Second
	parentPollInterval = 250 * time.Millisecond
	applyTimeout       = 5 * time.Minute
)

var errParentStillRunning = errors.New("selfupdate: launcher did not exit before the timeout")

func writeUpdateSpec(path string, spec updateSpec) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create selfupdate spec dir: %w", err)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode selfupdate spec: %w", err)
	}
	if err := storage.WriteAtomic(path, data); err != nil {
		return fmt.Errorf("write selfupdate spec: %w", err)
	}
	return nil
}

func readUpdateSpec(path string) (updateSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return updateSpec{}, fmt.Errorf("read selfupdate spec: %w", err)
	}
	var spec updateSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return updateSpec{}, fmt.Errorf("decode selfupdate spec: %w", err)
	}
	return spec, nil
}
