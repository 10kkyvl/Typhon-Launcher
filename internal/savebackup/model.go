package savebackup

import (
	"time"

	"typhon/internal/uierr"
)

type Kind string

const (
	KindManual     Kind = "manual"
	KindSession    Kind = "session"
	KindUpdate     Kind = "update"
	KindPreRestore Kind = "pre-restore"
)

func (k Kind) valid() bool {
	switch k {
	case KindManual, KindSession, KindUpdate, KindPreRestore:
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
	errInvalidID        = uierr.New("savebackup.invalid_id", "некорректный идентификатор")
	errInvalidKind      = uierr.New("savebackup.invalid_kind", "неизвестный вид резервной копии")
	errSavesNotFound    = uierr.New("savebackup.saves_not_found", "папка сохранений не найдена")
	errSavesAmbiguous   = uierr.New("savebackup.saves_ambiguous", "найдено несколько папок сохранений, выберите нужную")
	errSavesNotDir      = uierr.New("savebackup.saves_not_a_directory", "путь сохранений не является папкой")
	errNoSource         = uierr.New("savebackup.no_source", "не указана папка, которую нужно скопировать")
	errGameRunning      = uierr.New("savebackup.game_running", "игра запущена — закройте её перед восстановлением сохранений")
	errSnapshotNotFound = uierr.New("savebackup.snapshot_not_found", "резервная копия не найдена")
	errSnapshotBroken   = uierr.New("savebackup.snapshot_broken", "резервная копия повреждена")
	errVerify           = uierr.New("savebackup.verify_failed", "копия сохранений не совпала с оригиналом")
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
