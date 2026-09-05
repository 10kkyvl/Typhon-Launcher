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
	ErrBusy                = uierr.New("selfupdate.busy", "selfupdate: another update operation is in progress")
	ErrReadOnly            = uierr.New("selfupdate.read_only", "selfupdate: state failed to load, refusing to persist")
	ErrNotReplaced         = uierr.New("selfupdate.not_replaced", "selfupdate: installer finished but left the launcher binary unchanged")
	ErrManifestOutdated    = uierr.New("selfupdate.manifest_outdated", "selfupdate: launcher version is rejected by the manifest endpoint")

	ErrInvalidReleaseNote    = uierr.New("selfupdate.invalid_release_note", "selfupdate: release note is malformed")
	ErrInvalidChangeKind     = uierr.New("selfupdate.invalid_change_kind", "selfupdate: release note change kind is unknown")
	ErrEmptyNoteText         = uierr.New("selfupdate.empty_note_text", "selfupdate: release note text is empty")
	ErrInvalidNoteText       = uierr.New("selfupdate.invalid_note_text", "selfupdate: release note text is unsafe")
	ErrTooManyReleaseNotes   = uierr.New("selfupdate.too_many_release_notes", "selfupdate: release notes exceed the entry limit")
	ErrUnorderedReleaseNotes = uierr.New("selfupdate.unordered_release_notes", "selfupdate: release notes are not ordered newest first")
)
