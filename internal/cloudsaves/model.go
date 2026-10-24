// Package cloudsaves keeps one archive of every installed game's saves in the
// player's Typhon account. It acts only while the player has switched the
// setting on and is signed in, and it never overwrites either side silently:
// a change on both sides is recorded as a conflict and left for the player.
package cloudsaves

import (
	"time"

	"typhon/internal/uierr"
)

const EventName = "saves:cloud"

type State string

const (
	StateSynced   State = "synced"
	StatePending  State = "pending"
	StateConflict State = "conflict"
	StateFailed   State = "failed"
	StateSkipped  State = "skipped"
)

// Event is the saves:cloud payload. GameID is the library id of the game and
// is empty for a failure that concerns the whole account.
type Event struct {
	GameID     string `json:"gameId"`
	Status     State  `json:"status"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	UsedBytes  int64  `json:"usedBytes"`
	QuotaBytes int64  `json:"quotaBytes"`
}

// Conflict describes a game whose saves changed on this device and in the
// cloud since the last time they agreed.
type Conflict struct {
	LocalDigest     string    `json:"localDigest"`
	LocalUpdatedAt  time.Time `json:"localUpdatedAt"`
	LocalSize       int64     `json:"localSize"`
	RemoteDigest    string    `json:"remoteDigest"`
	RemoteUpdatedAt time.Time `json:"remoteUpdatedAt"`
	RemoteSize      int64     `json:"remoteSize"`
	RemoteDevice    string    `json:"remoteDevice"`
}

type GameStatus struct {
	GameID   string    `json:"gameId"`
	State    State     `json:"state"`
	Code     string    `json:"code"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
	SyncedAt time.Time `json:"syncedAt"`
	Conflict *Conflict `json:"conflict,omitempty"`
}

type Status struct {
	Enabled    bool         `json:"enabled"`
	SignedIn   bool         `json:"signedIn"`
	Pending    bool         `json:"pending"`
	UsedBytes  int64        `json:"usedBytes"`
	QuotaBytes int64        `json:"quotaBytes"`
	MaxBytes   int64        `json:"maxBytes"`
	Error      string       `json:"error"`
	Games      []GameStatus `json:"games"`
}

const (
	codeDisabled      = "cloud.disabled"
	codeNotSignedIn   = "cloud.not_signed_in"
	codeNotStarted    = "cloud.not_started"
	codeNoCanonicalID = "cloud.no_canonical_id"
	codeInvalidID     = "cloud.invalid_id"
	codeGameNotFound  = "cloud.game_not_found"
	codeStateBroken   = "cloud.state_unreadable"
	codeAccountUnk    = "cloud.account_unknown"
	codeToken         = "cloud.token"
	codeSymlink       = "cloud.symlink"
	codeIrregular     = "cloud.irregular_file"
	codeTooLarge      = "cloud.too_large"
	codeQuota         = "cloud.quota"
	codeInvalid       = "cloud.invalid_archive"
	codeDigest        = "cloud.digest_mismatch"
	codeEmpty         = "cloud.empty"
	codeExcluded      = "cloud.excluded"
	codeDuplicate     = "cloud.duplicate"
	codeNoRemote      = "cloud.no_remote"
	codeSavesNotDir   = "cloud.saves_not_a_directory"
	codeSavesMissing  = "cloud.saves_not_found"
	codeSavesAmbig    = "cloud.saves_ambiguous"
	codeSavesCache    = "cloud.saves_cache_like"
	codeSavesUnread   = "cloud.saves_unreadable"
	codeGameRunning   = "cloud.game_running"
	codeNoConflict    = "cloud.no_conflict"
	codeRemoteMissing = "cloud.remote_missing"
	codeNetwork       = "cloud.network"
	codeUnauthorized  = "cloud.unauthorized"
	codeRateLimited   = "cloud.rate_limited"
	codeBlocked       = "cloud.blocked"
	codeServer        = "cloud.server"
	codeConflictHTTP  = "cloud.conflict"
	codeCancelled     = "cloud.cancelled"
	codeFailed        = "cloud.failed"
	codeRemoved       = "cloud.removed"
	codeUnknownLimit  = "cloud.unknown_limit"
	codeBackup        = "cloud.backup_failed"
	codeBusy          = "cloud.busy"
	codeConflictRetry = "cloud.conflict_retry"
)

var (
	errDisabled      = uierr.New(codeDisabled, "облачные сохранения выключены")
	errNotSignedIn   = uierr.New(codeNotSignedIn, "войдите в аккаунт, чтобы синхронизировать сохранения")
	errNotStarted    = uierr.New(codeNotStarted, "сервис облачных сохранений не запущен")
	errNoCanonicalID = uierr.New(codeNoCanonicalID, "игра не опознана в каталоге, облачные сохранения для неё недоступны")
	errInvalidID     = uierr.New(codeInvalidID, "некорректный идентификатор игры в каталоге")
	errGameNotFound  = uierr.New(codeGameNotFound, "игра не найдена в библиотеке")
	errStateBroken   = uierr.New(codeStateBroken, "файл состояния облачных сохранений не читается, синхронизация остановлена")
	errAccountUnk    = uierr.New(codeAccountUnk, "аккаунт ещё не определён, повторите позже")
	errSymlink       = uierr.New(codeSymlink, "в папке сохранений есть символическая ссылка")
	errIrregular     = uierr.New(codeIrregular, "в папке сохранений есть файл особого типа")
	errTooLarge      = uierr.New(codeTooLarge, "сохранения не помещаются в лимит облака")
	errInvalid       = uierr.New(codeInvalid, "архив сохранений некорректен")
	errDigest        = uierr.New(codeDigest, "содержимое архива не совпало с контрольной суммой")
	errEmpty         = uierr.New(codeEmpty, "в папке сохранений нет файлов")
	errExcluded      = uierr.New(codeExcluded, "облачная копия удалена, игра исключена из синхронизации до ручного запуска")
	errDuplicate     = uierr.New(codeDuplicate, "в библиотеке несколько записей этой игры, синхронизация пропущена")
	errNoRemote      = uierr.New(codeNoRemote, "в облаке нет сохранений этой игры")
	errSavesNotDir   = uierr.New(codeSavesNotDir, "путь сохранений не является папкой")
	errSavesMissing  = uierr.New(codeSavesMissing, "папка сохранений не найдена")
	errSavesAmbig    = uierr.New(codeSavesAmbig, "найдено несколько папок сохранений, выберите нужную")
	errSavesCache    = uierr.New(codeSavesCache, "найденная папка похожа на кеш игры, а не на сохранения")
	errSavesUnread   = uierr.New(codeSavesUnread, "папку сохранений не удалось прочитать")
	errGameRunning   = uierr.New(codeGameRunning, "игра запущена, синхронизация подождёт")
	errNoConflict    = uierr.New(codeNoConflict, "у игры нет конфликта сохранений")
	errRemoteMissing = uierr.New(codeRemoteMissing, "в облаке нет сохранений этой игры")
	errConflictRetry = uierr.New(codeConflictRetry, "сервер отклонил загрузку сохранений дважды, попробуйте позже")
	errUnknownLimit  = uierr.New(codeUnknownLimit, "сервер не сообщил лимит размера сохранений")
)
