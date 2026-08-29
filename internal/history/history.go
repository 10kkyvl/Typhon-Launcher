package history

import (
	"errors"
	"strings"
	"time"
)

type Kind string

const (
	KindInstalled     Kind = "installed"
	KindInstallFailed Kind = "install_failed"
	KindUpdated       Kind = "updated"
	KindUpdateFailed  Kind = "update_failed"
	KindRolledBack    Kind = "rolled_back"
	KindDownloaded    Kind = "downloaded"
	KindUninstalled   Kind = "uninstalled"
	KindRemoved       Kind = "removed"
	KindMoved         Kind = "moved"
	KindLanReceived   Kind = "lan_received"
)

type Record struct {
	ID          string    `json:"id"`
	Kind        Kind      `json:"kind"`
	At          time.Time `json:"at"`
	GameID      string    `json:"gameId,omitempty"`
	Title       string    `json:"title"`
	FromVersion string    `json:"fromVersion,omitempty"`
	ToVersion   string    `json:"toVersion,omitempty"`
	Bytes       int64     `json:"bytes,omitempty"`
	BytesKnown  bool      `json:"bytesKnown"`
	Detail      string    `json:"detail,omitempty"`
	RefID       string    `json:"refId,omitempty"`
}

type Filter struct {
	Kinds []Kind
	Query string
	Limit int
}

type Status struct {
	Degraded bool   `json:"degraded"`
	Message  string `json:"message"`
}

var (
	ErrBadKind    = errors.New("неизвестный тип записи журнала")
	ErrEmptyTitle = errors.New("не указано название")
)

const (
	maxRecords = 500
	maxAge     = 180 * 24 * time.Hour
)

func validKind(k Kind) bool {
	switch k {
	case KindInstalled, KindInstallFailed, KindUpdated, KindUpdateFailed,
		KindRolledBack, KindDownloaded, KindUninstalled, KindRemoved,
		KindMoved, KindLanReceived:
		return true
	default:
		return false
	}
}

// trimRecords enforces both retention bounds. Records are stored oldest
// first, so records past maxAge are always a prefix of the slice.
func trimRecords(records []Record, now time.Time) []Record {
	cutoff := now.Add(-maxAge)
	start := 0
	for start < len(records) && records[start].At.Before(cutoff) {
		start++
	}
	records = records[start:]
	if len(records) > maxRecords {
		records = records[len(records)-maxRecords:]
	}
	return records
}

func matchesFilter(r Record, f Filter) bool {
	if len(f.Kinds) > 0 {
		found := false
		for _, k := range f.Kinds {
			if r.Kind == k {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	query := strings.TrimSpace(f.Query)
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	return strings.Contains(strings.ToLower(r.Title), query) ||
		strings.Contains(strings.ToLower(r.Detail), query)
}
