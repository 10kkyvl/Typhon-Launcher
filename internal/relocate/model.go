package relocate

import (
	"errors"
	"time"
)

type Scope string

const (
	ScopeGame    Scope = "game"
	ScopeLibrary Scope = "library"
)

type Stage string

const (
	StagePrepare   Stage = "prepare"
	StageCopy      Stage = "copy"
	StageVerify    Stage = "verify"
	StageCommit    Stage = "commit"
	StageRepoint   Stage = "repoint"
	StageCleanup   Stage = "cleanup"
	StageDone      Stage = "done"
	StageFailed    Stage = "failed"
	StageCancelled Stage = "cancelled"
)

const (
	phaseCopying   = "копирование"
	phaseVerifying = "проверка"
)

// itemDownloads and itemScreenshots are pseudo game IDs used as Queue/GameID
// values while a library move works through the two directories that are
// not tied to a single game.
const (
	itemDownloads   = "@downloads"
	itemScreenshots = "@screenshots"
	itemSettings    = "@settings"
)

type Job struct {
	ID          string    `json:"id"`
	Scope       Scope     `json:"scope"`
	Stage       Stage     `json:"stage"`
	GameID      string    `json:"gameId,omitempty"`
	Title       string    `json:"title"`
	Source      string    `json:"source"`
	Target      string    `json:"target"`
	Staging     string    `json:"staging"`
	Renamed     bool      `json:"renamed"`
	TotalBytes  int64     `json:"totalBytes"`
	CopiedBytes int64     `json:"copiedBytes"`
	Phase       string    `json:"phase"`
	CurrentFile string    `json:"currentFile"`
	Queue       []string  `json:"queue,omitempty"`
	StartedAt   time.Time `json:"startedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Error       string    `json:"error,omitempty"`
}

func (j Job) clone() Job {
	out := j
	out.Queue = append([]string(nil), j.Queue...)
	return out
}

var (
	ErrEmptyInstallDir    = errors.New("у игры не задан каталог установки")
	ErrGameRunning        = errors.New("игра сейчас запущена")
	ErrUpdateBusy         = errors.New("для игры выполняется обновление")
	ErrInstallBusy        = errors.New("для игры выполняется установка")
	ErrDownloadBusy       = errors.New("для игры идёт загрузка")
	ErrEmptySource        = errors.New("не указан исходный каталог")
	ErrEmptyTarget        = errors.New("не указан целевой каталог")
	ErrRelativeTarget     = errors.New("целевой каталог должен быть абсолютным путём")
	ErrTargetIsRoot       = errors.New("целевой каталог не может быть корнем диска")
	ErrTargetInsideSource = errors.New("целевой каталог не может быть внутри исходного")
	ErrSourceInsideTarget = errors.New("исходный каталог не может быть внутри целевого")
	ErrTargetNotEmpty     = errors.New("целевой каталог не пуст или недоступен")
	ErrFreeSpaceUnknown   = errors.New("не удалось определить свободное место на диске")
	ErrNotEnoughSpace     = errors.New("недостаточно свободного места на диске")
	ErrVerifyFailed       = errors.New("проверка перенесённых файлов не прошла")
	ErrJobNotFound        = errors.New("операция переноса не найдена")
	ErrGameNotFound       = errors.New("игра не найдена")
	ErrAmbiguousRecovery  = errors.New("восстановление переноса неоднозначно, требуется вмешательство пользователя")
	ErrMoveInProgress     = errors.New("перенос уже выполняется")
)

// spaceMarginPercent is the headroom required on top of the measured
// transfer size before a move is allowed to start.
const spaceMarginPercent = 5
