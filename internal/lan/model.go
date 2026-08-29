package lan

import "time"

// Progress reports how far BuildInfo has advanced through hashing a shared
// tree. ProcessedBytes never decreases and reaches TotalBytes on success.
type Progress struct {
	ProcessedBytes int64
	TotalBytes     int64
	CurrentFile    string
}

// HashProgress is the wails-facing event payload for a share build in
// progress, keyed by the game being shared.
type HashProgress struct {
	GameID         string `json:"gameId"`
	ProcessedBytes int64  `json:"processedBytes"`
	TotalBytes     int64  `json:"totalBytes"`
	CurrentFile    string `json:"currentFile"`
	Done           bool   `json:"done"`
	Error          string `json:"error,omitempty"`
}

// Share is a locally shared, installed game: metadata plus the cached
// fingerprint that lets Share skip re-hashing an unchanged tree.
type Share struct {
	GameID      string    `json:"gameId"`
	Title       string    `json:"title"`
	Version     string    `json:"version"`
	Exe         string    `json:"exe"`
	Root        string    `json:"root"`
	InfoHash    string    `json:"infoHash"`
	SizeBytes   int64     `json:"sizeBytes"`
	Fingerprint string    `json:"fingerprint"`
	BuiltAt     time.Time `json:"builtAt"`
	Enabled     bool      `json:"enabled"`
}

// Peer is a machine on the LAN heard from recently, independent of whether
// it currently has anything on offer.
type Peer struct {
	ID       string    `json:"id"`
	Host     string    `json:"host"`
	Addr     string    `json:"addr"`
	Port     int       `json:"port"`
	LastSeen time.Time `json:"lastSeen"`
}

// Offer is one announced share from one peer, keyed by (PeerID, InfoHash).
type Offer struct {
	PeerID    string    `json:"peerId"`
	Host      string    `json:"host"`
	Addr      string    `json:"addr"`
	Port      int       `json:"port"`
	GameID    string    `json:"gameId"`
	Title     string    `json:"title"`
	Version   string    `json:"version"`
	Exe       string    `json:"exe"`
	SizeBytes int64     `json:"sizeBytes"`
	InfoHash  string    `json:"infoHash"`
	LastSeen  time.Time `json:"lastSeen"`
}

type TransferStatus string

const (
	TransferReceiving TransferStatus = "receiving"
	TransferCompleted TransferStatus = "completed"
	TransferFailed    TransferStatus = "failed"
	TransferCancelled TransferStatus = "cancelled"
)

// Transfer is an in-progress or finished receive of an Offer.
type Transfer struct {
	ID         string         `json:"id"`
	InfoHash   string         `json:"infoHash"`
	PeerID     string         `json:"peerId"`
	GameID     string         `json:"gameId"`
	Title      string         `json:"title"`
	Downloaded int64          `json:"downloaded"`
	Total      int64          `json:"total"`
	Status     TransferStatus `json:"status"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"startedAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

// Stats counts what the announce receiver saw and rejected, so the UI can
// tell "nothing arrived" from "arrived and was rejected".
type Stats struct {
	AnnouncesSent     int64            `json:"announcesSent"`
	AnnouncesReceived int64            `json:"announcesReceived"`
	Rejected          map[string]int64 `json:"rejected"`
	PeersKnown        int              `json:"peersKnown"`
	OffersKnown       int              `json:"offersKnown"`
	SharesActive      int              `json:"sharesActive"`
}
