package selfupdate

import "errors"

var (
	ErrBadPublicKey        = errors.New("selfupdate: embedded public key is invalid")
	ErrUnknownKey          = errors.New("selfupdate: manifest signed by an unknown key")
	ErrBadSignature        = errors.New("selfupdate: manifest signature does not verify")
	ErrManifestTooLarge    = errors.New("selfupdate: manifest exceeds the size limit")
	ErrInvalidManifest     = errors.New("selfupdate: manifest is malformed")
	ErrInvalidVersion      = errors.New("selfupdate: manifest version is not comparable")
	ErrUnsupportedKind     = errors.New("selfupdate: unsupported artifact kind")
	ErrInvalidArtifact     = errors.New("selfupdate: artifact is incomplete")
	ErrInvalidArtifactName = errors.New("selfupdate: artifact name is unsafe")
	ErrInvalidArtifactURL  = errors.New("selfupdate: artifact url is not https")
	ErrInvalidArtifactSize = errors.New("selfupdate: artifact size is out of range")
	ErrInvalidHash         = errors.New("selfupdate: artifact hash is not a sha-256 digest")
	ErrNoArtifact          = errors.New("selfupdate: manifest has no artifact for this platform")
	ErrSizeMismatch        = errors.New("selfupdate: downloaded size differs from the manifest")
	ErrHashMismatch        = errors.New("selfupdate: downloaded hash differs from the manifest")
	ErrNotReady            = errors.New("selfupdate: no verified update is ready to apply")
	ErrApplyUnsupported    = errors.New("selfupdate: applying updates is not supported on this platform")
	ErrBusy                = errors.New("selfupdate: another update operation is in progress")
	ErrReadOnly            = errors.New("selfupdate: state failed to load, refusing to persist")
	ErrNotReplaced         = errors.New("selfupdate: installer finished but left the launcher binary unchanged")
)
