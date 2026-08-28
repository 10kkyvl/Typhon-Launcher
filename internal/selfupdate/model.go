package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"typhon/internal/account"
	"typhon/internal/version"
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
	Version     string        `json:"version"`
	PublishedAt time.Time     `json:"publishedAt"`
	Notes       string        `json:"notes,omitempty"`
	Releases    []ReleaseNote `json:"releases,omitempty"`
	Artifacts   []Artifact    `json:"artifacts"`
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

func (s Status) MarshalJSON() ([]byte, error) {
	type alias Status
	out := struct {
		alias
		PublishedAt *time.Time `json:"publishedAt,omitempty"`
		CheckedAt   *time.Time `json:"checkedAt,omitempty"`
	}{alias: alias(s)}
	if !s.PublishedAt.IsZero() {
		out.PublishedAt = &s.PublishedAt
	}
	if !s.CheckedAt.IsZero() {
		out.CheckedAt = &s.CheckedAt
	}
	return json.Marshal(out)
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

type ChangeKind string

const (
	ChangeAdded   ChangeKind = "added"
	ChangeChanged ChangeKind = "changed"
	ChangeFixed   ChangeKind = "fixed"
	ChangeRemoved ChangeKind = "removed"
)

const (
	MaxReleaseNotes   = 30
	MaxChangesPerNote = 40
	MaxChangeLen      = 300
	MaxSummaryLen     = 300
)

type Change struct {
	Kind ChangeKind `json:"kind"`
	Text string     `json:"text"`
}

type ReleaseNote struct {
	Version     string    `json:"version"`
	PublishedAt time.Time `json:"publishedAt"`
	Summary     string    `json:"summary,omitempty"`
	Changes     []Change  `json:"changes"`
}

type ReleaseNotes struct {
	CurrentVersion string        `json:"currentVersion"`
	Unseen         []ReleaseNote `json:"unseen"`
	History        []ReleaseNote `json:"history"`
}

func (c Change) Validate() error {
	switch c.Kind {
	case ChangeAdded, ChangeChanged, ChangeFixed, ChangeRemoved:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidChangeKind, c.Kind)
	}
	return validateNoteText(c.Text, MaxChangeLen)
}

func (n ReleaseNote) Validate() error {
	if !version.Parse(n.Version).Comparable {
		return fmt.Errorf("%w: version %q", ErrInvalidReleaseNote, n.Version)
	}
	if n.PublishedAt.IsZero() {
		return fmt.Errorf("%w: %s has no publish date", ErrInvalidReleaseNote, n.Version)
	}
	if n.Summary != "" {
		if err := validateNoteText(n.Summary, MaxSummaryLen); err != nil {
			return err
		}
	}
	if len(n.Changes) == 0 || len(n.Changes) > MaxChangesPerNote {
		return fmt.Errorf("%w: %s has %d changes", ErrInvalidReleaseNote, n.Version, len(n.Changes))
	}
	for _, c := range n.Changes {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("%s: %w", n.Version, err)
		}
	}
	return nil
}

func validateNoteText(s string, max int) error {
	if s == "" {
		return ErrEmptyNoteText
	}
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return fmt.Errorf("%w: %d runes", ErrInvalidNoteText, utf8.RuneCountInString(s))
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return ErrInvalidNoteText
		}
	}
	return nil
}

func validateReleaseNotes(notes []ReleaseNote) error {
	if len(notes) > MaxReleaseNotes {
		return fmt.Errorf("%w: %d entries", ErrTooManyReleaseNotes, len(notes))
	}
	for i, n := range notes {
		if err := n.Validate(); err != nil {
			return err
		}
		if i == 0 {
			continue
		}
		newer, err := IsNewer(notes[i-1].Version, n.Version)
		if err != nil {
			return err
		}
		if !newer {
			return fmt.Errorf("%w: %q does not precede %q", ErrUnorderedReleaseNotes, notes[i-1].Version, n.Version)
		}
	}
	return nil
}
