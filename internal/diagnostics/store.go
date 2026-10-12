package diagnostics

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"typhon/internal/storage"
)

const (
	pendingVersion  = 1
	maxPendingFiles = 20

	serverMaxReportAge   = 30 * 24 * time.Hour
	serverMaxReportAhead = 24 * time.Hour
	reportWindowSlack    = time.Hour
)

// withinServerWindow drops what the server would refuse for its timestamp. The
// server rejects the whole batch over one such report, so a file with a stale
// report would otherwise be quarantined together with its valid neighbours.
// The slack keeps a report that is on the edge from expiring in transit.
func withinServerWindow(batch []reportPayload, now time.Time) (kept []reportPayload, dropped int) {
	oldest := now.Add(-serverMaxReportAge + reportWindowSlack)
	newest := now.Add(serverMaxReportAhead - reportWindowSlack)
	kept = make([]reportPayload, 0, len(batch))
	for _, r := range batch {
		if r.Timestamp.IsZero() || r.Timestamp.Before(oldest) || r.Timestamp.After(newest) {
			dropped++
			continue
		}
		kept = append(kept, r)
	}
	return kept, dropped
}

// sendablePending applies the rules of a fresh capture to reports read back
// from disk: files written by an older launcher, or before an update to these
// rules, would otherwise reach the server unchecked and be refused as a whole.
func sendablePending(batch []reportPayload, now time.Time) (kept []reportPayload, dropped int) {
	batch, dropped = withinServerWindow(batch, now)
	kept = make([]reportPayload, 0, len(batch))
	for _, r := range batch {
		clean, err := sanitizeReport(Report(r))
		if err != nil {
			dropped++
			continue
		}
		kept = append(kept, toPayload(clean))
	}
	return kept, dropped
}

func pendingDirFrom(configDir string) string {
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "diagnostics", "pending")
}

func pendingFilePath(dir string, now time.Time) (string, error) {
	ts := now.UnixNano()
	for {
		path := filepath.Join(dir, strconv.FormatInt(ts, 10)+".json")
		_, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
		ts++
	}
}

func listPendingFiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Slice(names, func(i, j int) bool {
		return pendingTimestamp(names[i]) < pendingTimestamp(names[j])
	})
	return names, nil
}

func pendingTimestamp(name string) int64 {
	ts, err := strconv.ParseInt(strings.TrimSuffix(name, ".json"), 10, 64)
	if err != nil {
		return 0
	}
	return ts
}

func savePending(dir string, now time.Time, batch []reportPayload) error {
	if dir == "" {
		return errors.New("diagnostics pending dir unavailable")
	}
	names, err := listPendingFiles(dir)
	if err != nil {
		return fmt.Errorf("list pending diagnostics: %w", err)
	}
	for len(names) >= maxPendingFiles {
		oldest := filepath.Join(dir, names[0])
		if err := os.Remove(oldest); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("evict pending diagnostics: %w", err)
		}
		names = names[1:]
	}
	path, err := pendingFilePath(dir, now)
	if err != nil {
		return fmt.Errorf("allocate pending diagnostics path: %w", err)
	}
	return storage.Save(path, pendingVersion, batch)
}

func loadPending(path string) ([]reportPayload, error) {
	var batch []reportPayload
	if err := storage.Load(path, pendingVersion, nil, &batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func removePendingDir(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove pending diagnostics dir: %w", err)
	}
	return nil
}
