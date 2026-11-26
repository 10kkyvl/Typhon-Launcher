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
	// TitleGuessed: see library.ExternalGame.TitleGuessed.
	TitleGuessed bool
}

type Skipped struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
	AppID  string `json:"-"`
}

type ProviderScan struct {
	Provider string
	Games    []Candidate
	Skipped  []Skipped
	NotGames []string
	Err      error
}
