package savebackup

import (
	"errors"
	"time"

	"typhon/internal/uierr"
)

type Kind string

const (
	KindManual     Kind = "manual"
	KindSession    Kind = "session"
	KindUpdate     Kind = "update"
	KindPreRestore Kind = "pre-restore"
	KindCloud      Kind = "cloud"
	KindLaunch     Kind = "launch"
)

func (k Kind) valid() bool {
	switch k {
	case KindManual, KindSession, KindUpdate, KindPreRestore, KindCloud, KindLaunch:
		return true
	}
	return false
}

type Status string

const (
	StatusCreated  Status = "created"
	StatusSkipped  Status = "skipped"
	StatusFailed   Status = "failed"
	StatusDeleted  Status = "deleted"
	StatusRestored Status = "restored"
)

const EventName = "saves:backups"

// Snapshot is one finished copy of a game's saves. A snapshot whose metadata
// or files cannot be trusted is still listed, with Broken set, so the player
// can see and delete it instead of the list quietly shrinking.
type Snapshot struct {
	ID         string    `json:"id"`
	GameID     string    `json:"gameId"`
	Kind       Kind      `json:"kind"`
	CreatedAt  time.Time `json:"createdAt"`
	SourcePath string    `json:"sourcePath"`
	SizeBytes  int64     `json:"sizeBytes"`
	Files      int       `json:"files"`
	Digest     string    `json:"digest"`
	Path       string    `json:"path,omitempty"`
	Broken     bool      `json:"broken,omitempty"`
	Problem    string    `json:"problem,omitempty"`
	// Large marks saves above largeSavesBytes: the copy exists, but each
	// automatic one of that size costs the disk and the time before a launch.
	// It is derived from SizeBytes on every read, not stored.
	Large bool `json:"large"`
	// PlayedAfterSeconds is the playing time recorded after CreatedAt. It is
	// filled by List only; PlayedAfterKnown is false when the play log could
	// not answer, and then the number means nothing.
	PlayedAfterSeconds int64 `json:"playedAfterSeconds"`
	PlayedAfterKnown   bool  `json:"playedAfterKnown"`
}

// Event is the saves:backups payload. Error is set next to StatusCreated when
// the copy exists but trimming old copies failed.
type Event struct {
	GameID   string    `json:"gameId"`
	Kind     Kind      `json:"kind"`
	Snapshot *Snapshot `json:"snapshot"`
	Status   Status    `json:"status"`
	Code     string    `json:"code"`
	Error    string    `json:"error"`
}

var (
	errNotStarted       = uierr.New("savebackup.not_started", "сервис резервных копий сохранений не запущен")
	errDisabled         = uierr.New("savebackup.disabled", "резервные копии сохранений пока недоступны")
	errInvalidID        = uierr.New("savebackup.invalid_id", "некорректный идентификатор")
	errInvalidKind      = uierr.New("savebackup.invalid_kind", "неизвестный вид резервной копии")
	errSavesNotFound    = uierr.New("savebackup.saves_not_found", "папка сохранений не найдена")
	errSavesAmbiguous   = uierr.New("savebackup.saves_ambiguous", "найдено несколько папок сохранений, выберите нужную")
	errSavesCacheLike   = uierr.New("savebackup.saves_cache_like", "найденная папка похожа на кеш игры, а не на сохранения")
	errSavesNotDir      = uierr.New("savebackup.saves_not_a_directory", "путь сохранений не является папкой")
	errNoSource         = uierr.New("savebackup.no_source", "не указана папка, которую нужно скопировать")
	errGameRunning      = uierr.New("savebackup.game_running", "игра запущена — закройте её перед восстановлением сохранений")
	errSnapshotNotFound = uierr.New("savebackup.snapshot_not_found", "резервная копия не найдена")
	errSnapshotBroken   = uierr.New("savebackup.snapshot_broken", "резервная копия повреждена")
	errVerify           = uierr.New("savebackup.verify_failed", "копия сохранений не совпала с оригиналом")
	errSavesUnreadable  = errors.New("часть каталогов не удалось прочитать")
	errSavesHintsFailed = errors.New("подсказки путей сохранений недоступны")
	errRecoveryPending  = uierr.New("savebackup.recovery_pending", "прерванный откат сохранений не завершён — запуск остановлен, чтобы не потерять сохранения")
	errLaunchTimeout    = uierr.New("savebackup.launch_timeout", "копия сохранений перед запуском не успела — игра запущена без неё")
	errUnchanged        = uierr.New("savebackup.unchanged", "сохранения не изменились с прошлой копии")
	errLeftovers        = uierr.New("savebackup.restore_leftovers", "рядом с папкой сохранений остались следы прерванного восстановления")
	errRecovery         = uierr.New("savebackup.recovery_failed", "не удалось довести до конца прерванную операцию с сохранениями")
)

const (
	codeNoFreeSpace      = "savebackup.no_free_space"
	codeRotationFailed   = "savebackup.rotation_failed"
	codeCleanupFailed    = "savebackup.cleanup_failed"
	codeSavesUnavailable = "savebackup.saves_path_unavailable"
	codeRecoveryFailed   = "savebackup.recovery_failed"
)
