package relocate

import (
	"time"
	"typhon/internal/uierr"
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
	ErrEmptyInstallDir    = uierr.New("relocate.no_install_dir", "у игры не задан каталог установки")
	ErrGameRunning        = uierr.New("relocate.game_running", "игра сейчас запущена")
	ErrUpdateBusy         = uierr.New("relocate.updating", "для игры выполняется обновление")
	ErrInstallBusy        = uierr.New("relocate.installing", "для игры выполняется установка")
	ErrDownloadBusy       = uierr.New("relocate.downloading", "для игры идёт загрузка")
	ErrEmptySource        = uierr.New("relocate.no_source", "не указан исходный каталог")
	ErrEmptyTarget        = uierr.New("relocate.no_target", "не указан целевой каталог")
	ErrRelativeTarget     = uierr.New("relocate.invalid_path", "целевой каталог должен быть абсолютным путём")
	ErrTargetIsRoot       = uierr.New("relocate.target_is_drive_root", "целевой каталог не может быть корнем диска")
	ErrTargetInsideSource = uierr.New("relocate.target_inside_source", "целевой каталог не может быть внутри исходного")
	ErrSourceInsideTarget = uierr.New("relocate.source_inside_target", "исходный каталог не может быть внутри целевого")
	ErrTargetNotEmpty     = uierr.New("relocate.target_not_empty", "целевой каталог не пуст или недоступен")
	ErrFreeSpaceUnknown   = uierr.New("relocate.free_space_unknown", "не удалось определить свободное место на диске")
	ErrNotEnoughSpace     = uierr.New("relocate.not_enough_space", "недостаточно свободного места на диске")
	ErrVerifyFailed       = uierr.New("relocate.verify_failed", "проверка перенесённых файлов не прошла")
	ErrJobNotFound        = uierr.New("relocate.job_not_found", "операция переноса не найдена")
	ErrGameNotFound       = uierr.New("relocate.game_not_found", "игра не найдена")
	ErrAmbiguousRecovery  = uierr.New("relocate.ambiguous_recovery", "восстановление переноса неоднозначно, требуется вмешательство пользователя")
	ErrMoveInProgress     = uierr.New("relocate.already_running", "перенос уже выполняется")
)

// spaceMarginPercent is the headroom required on top of the measured
// transfer size before a move is allowed to start.
const spaceMarginPercent = 5
