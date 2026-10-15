package extlib

import (
	"errors"
	"fmt"
)

var (
	ErrUnsupported  = fmt.Errorf("импорт игр из других лаунчеров поддерживается только в Windows: %w", errors.ErrUnsupported)
	ErrNotInstalled = errors.New("лаунчер не установлен")
)

type Candidate struct {
	Provider   string
	AppID      string
	Title      string
	InstallDir string
	Executable string
	LaunchURI  string
}

type Skipped struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

type ProviderScan struct {
	Provider string
	Games    []Candidate
	Skipped  []Skipped
	Err      error
}
