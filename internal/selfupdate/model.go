package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"path"
	"strings"
	"time"

	"typhon/internal/account"
)

const (
	KeyID           = "typhon-release-1"
	ManifestPath    = "/launcher/manifest"
	MaxManifestSize = 64 << 10
	MaxArtifactSize = 512 << 20
	StateVersion    = 1
)

const publicKeyBase64 = "qGMobVJXZf8EdvQ8qCGsJrTko5LKkYoH8qlTBD9iCx4="

type State string

const (
	StateIdle        State = "idle"
	StateChecking    State = "checking"
	StateAvailable   State = "available"
	StateDownloading State = "downloading"
	StateReady       State = "ready"
	StateApplying    State = "applying"
	StateFailed      State = "failed"
)

type Kind string

const KindInstaller Kind = "installer"

type Artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version     string     `json:"version"`
	PublishedAt time.Time  `json:"publishedAt"`
	Notes       string     `json:"notes,omitempty"`
	Artifacts   []Artifact `json:"artifacts"`
}

type SignedManifest struct {
	KeyID     string          `json:"keyId"`
	Signature string          `json:"signature"`
	Manifest  json.RawMessage `json:"manifest"`
}

type Status struct {
	State            State     `json:"state"`
	CurrentVersion   string    `json:"currentVersion"`
	AvailableVersion string    `json:"availableVersion,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	PublishedAt      time.Time `json:"publishedAt,omitempty"`
	TotalBytes       int64     `json:"totalBytes,omitempty"`
	DownloadedBytes  int64     `json:"downloadedBytes,omitempty"`
	CheckedAt        time.Time `json:"checkedAt,omitempty"`
	Error            string    `json:"error,omitempty"`
	ErrorCode        string    `json:"errorCode,omitempty"`
}

type Progress struct {
	Version         string `json:"version"`
	TotalBytes      int64  `json:"totalBytes"`
	DownloadedBytes int64  `json:"downloadedBytes"`
}

type stored struct {
	AvailableVersion string    `json:"availableVersion,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	PublishedAt      time.Time `json:"publishedAt,omitempty"`
	CheckedAt        time.Time `json:"checkedAt,omitempty"`
	Artifact         *Artifact `json:"artifact,omitempty"`
	ReadyPath        string    `json:"readyPath,omitempty"`
}

func PublicKey() (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return nil, ErrBadPublicKey
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, ErrBadPublicKey
	}
	return ed25519.PublicKey(raw), nil
}

func (a Artifact) Validate() error {
	switch a.Kind {
	case KindInstaller:
	default:
		return ErrUnsupportedKind
	}
	if a.OS == "" || a.Arch == "" {
		return ErrInvalidArtifact
	}
	if err := validateArtifactName(a.Name); err != nil {
		return err
	}
	if err := validateArtifactURL(a.URL); err != nil {
		return err
	}
	if a.Size <= 0 || a.Size > MaxArtifactSize {
		return ErrInvalidArtifactSize
	}
	if err := validateHash(a.SHA256); err != nil {
		return err
	}
	return nil
}

func validateArtifactName(name string) error {
	if name == "" || len(name) > 128 {
		return ErrInvalidArtifactName
	}
	if name != path.Clean(name) || name == "." || name == ".." {
		return ErrInvalidArtifactName
	}
	if strings.ContainsAny(name, `/\:*?"<>|`) {
		return ErrInvalidArtifactName
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return ErrInvalidArtifactName
		}
	}
	return nil
}

func validateArtifactURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return ErrInvalidArtifactURL
	}
	if u.Host == "" {
		return ErrInvalidArtifactURL
	}
	if err := account.CheckURLScheme(u); err != nil {
		return ErrInvalidArtifactURL
	}
	return nil
}

func validateHash(h string) error {
	if len(h) != 64 {
		return ErrInvalidHash
	}
	for _, r := range h {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return ErrInvalidHash
		}
	}
	return nil
}
