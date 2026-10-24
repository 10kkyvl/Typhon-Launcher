package selfupdate

import (
	"errors"

	"typhon/internal/uierr"
)

// ErrBadPrivateKey stays a plain error: SignManifest runs only in the
// offline release-signing tools (cmd/signrelease, tools/devrelease), never
// through a wails-bound method, so it never reaches the UI.
var ErrBadPrivateKey = errors.New("selfupdate: private key is not an ed25519 private key")

var (
	ErrPortableUpdateUnavailable = uierr.New("selfupdate.portable_update_unavailable", "selfupdate: в этом выпуске нет портативной сборки")
	ErrPortableZipUnsafeEntry    = uierr.New("selfupdate.portable_zip_unsafe_entry", "selfupdate: в архиве портативной сборки небезопасное имя файла")
	ErrPortableZipNoExecutable   = uierr.New("selfupdate.portable_zip_no_executable", "selfupdate: в архиве портативной сборки нет typhon.exe")
	ErrPortableZipExecutable     = uierr.New("selfupdate.portable_zip_executable", "selfupdate: typhon.exe в архиве не обычный файл допустимого размера")
)

var (
	ErrBadPublicKey        = uierr.New("selfupdate.bad_public_key", "selfupdate: embedded public key is invalid")
	ErrUnknownKey          = uierr.New("selfupdate.unknown_key", "selfupdate: manifest signed by an unknown key")
	ErrBadSignature        = uierr.New("selfupdate.bad_signature", "selfupdate: manifest signature does not verify")
	ErrManifestTooLarge    = uierr.New("selfupdate.manifest_too_large", "selfupdate: manifest exceeds the size limit")
	ErrInvalidManifest     = uierr.New("selfupdate.invalid_manifest", "selfupdate: manifest is malformed")
	ErrInvalidVersion      = uierr.New("selfupdate.invalid_version", "selfupdate: manifest version is not comparable")
	ErrUnsupportedKind     = uierr.New("selfupdate.unsupported_artifact_kind", "selfupdate: unsupported artifact kind")
	ErrInvalidArtifact     = uierr.New("selfupdate.invalid_artifact", "selfupdate: artifact is incomplete")
	ErrInvalidArtifactName = uierr.New("selfupdate.invalid_artifact_name", "selfupdate: artifact name is unsafe")
	ErrInvalidArtifactURL  = uierr.New("selfupdate.invalid_artifact_url", "selfupdate: artifact url is not https")
	ErrInvalidArtifactSize = uierr.New("selfupdate.invalid_artifact_size", "selfupdate: artifact size is out of range")
	ErrInvalidHash         = uierr.New("selfupdate.invalid_hash", "selfupdate: artifact hash is not a sha-256 digest")
	ErrNoArtifact          = uierr.New("selfupdate.no_artifact", "selfupdate: manifest has no artifact for this platform")
	ErrStalled             = uierr.New("selfupdate.stalled", "selfupdate: artifact download stalled")
	ErrSizeMismatch        = uierr.New("selfupdate.size_mismatch", "selfupdate: downloaded size differs from the manifest")
	ErrHashMismatch        = uierr.New("selfupdate.hash_mismatch", "selfupdate: downloaded hash differs from the manifest")
	ErrNotReady            = uierr.New("selfupdate.not_ready", "selfupdate: no verified update is ready to apply")
	ErrApplyUnsupported    = uierr.New("selfupdate.apply_unsupported", "selfupdate: applying updates is not supported on this platform")
	ErrNoStableOffer       = uierr.New("selfupdate.no_stable_offer", "selfupdate: no stable release is offered to this beta build")
	ErrBusy                = uierr.New("selfupdate.busy", "selfupdate: another update operation is in progress")
	ErrReadOnly            = uierr.New("selfupdate.read_only", "selfupdate: state failed to load, refusing to persist")
	ErrNotReplaced         = uierr.New("selfupdate.not_replaced", "selfupdate: installer finished but left the launcher binary unchanged")
	ErrManifestOutdated    = uierr.New("selfupdate.manifest_outdated", "selfupdate: launcher version is rejected by the manifest endpoint")

	ErrRollbackUnsupported   = uierr.New("selfupdate.rollback_unsupported", "selfupdate: rolling back the launcher is not supported on this platform")
	ErrNoPrevious            = uierr.New("selfupdate.rollback_none", "selfupdate: no previous launcher version is saved")
	ErrPreviousBroken        = uierr.New("selfupdate.rollback_broken", "selfupdate: the saved previous launcher version is damaged")
	ErrPreviousHash          = uierr.New("selfupdate.rollback_hash", "selfupdate: the saved previous launcher differs from its record")
	ErrPreviousStateHash     = uierr.New("selfupdate.rollback_state_hash", "selfupdate: a saved state file differs from its record")
	ErrPreviousNotOlder      = uierr.New("selfupdate.rollback_not_older", "selfupdate: the saved launcher version is not older than the running one")
	ErrPreviousVersionDiffer = uierr.New("selfupdate.rollback_version_differs", "selfupdate: the saved launcher version is not the one requested")
	ErrSnapshotVersion       = uierr.New("selfupdate.snapshot_version", "selfupdate: the running launcher version is unknown, cannot save it for a rollback")

	ErrInvalidReleaseNote    = uierr.New("selfupdate.invalid_release_note", "selfupdate: release note is malformed")
	ErrInvalidChangeKind     = uierr.New("selfupdate.invalid_change_kind", "selfupdate: release note change kind is unknown")
	ErrEmptyNoteText         = uierr.New("selfupdate.empty_note_text", "selfupdate: release note text is empty")
	ErrInvalidNoteText       = uierr.New("selfupdate.invalid_note_text", "selfupdate: release note text is unsafe")
	ErrTooManyReleaseNotes   = uierr.New("selfupdate.too_many_release_notes", "selfupdate: release notes exceed the entry limit")
	ErrUnorderedReleaseNotes = uierr.New("selfupdate.unordered_release_notes", "selfupdate: release notes are not ordered newest first")
)
