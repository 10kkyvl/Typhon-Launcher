package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	hashBufferSize = 1 << 20
	minWorkers     = 2
	maxWorkers     = 4
	progressPeriod = 250 * time.Millisecond
)

type Progress struct {
	ProcessedBytes int64  `json:"processedBytes"`
	TotalBytes     int64  `json:"totalBytes"`
	CurrentFile    string `json:"currentFile"`
}

type IssueKind string

const (
	IssueMissing    IssueKind = "missing"
	IssueCorrupted  IssueKind = "corrupted"
	IssueSize       IssueKind = "size"
	IssueUnreadable IssueKind = "unreadable"
)

type ManifestIssue struct {
	Path string    `json:"path"`
	Kind IssueKind `json:"kind"`
}

type ManifestResult struct {
	TotalFiles int             `json:"totalFiles"`
	TotalBytes int64           `json:"totalBytes"`
	OkFiles    int             `json:"okFiles"`
	OkBytes    int64           `json:"okBytes"`
	Issues     []ManifestIssue `json:"issues"`
	Extra      []string        `json:"extra"`
}

// Count reports how many issues of one kind the check produced. Unreadable
// files are not damage: they are files the check could not look at.
func (r ManifestResult) Count(kinds ...IssueKind) int {
	n := 0
	for _, issue := range r.Issues {
		for _, kind := range kinds {
			if issue.Kind == kind {
				n++
				break
			}
		}
	}
	return n
}

func (r ManifestResult) Ratio() float64 {
	if r.TotalBytes <= 0 {
		return 0
	}
	if r.OkBytes >= r.TotalBytes {
		return 1
	}
	return float64(r.OkBytes) / float64(r.TotalBytes)
}

func workerCount() int {
	n := runtime.NumCPU() / 2
	if n < minWorkers {
		return minWorkers
	}
	if n > maxWorkers {
		return maxWorkers
	}
	return n
}

func hashFile(ctx context.Context, path string, buf []byte) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	sum := sha256.New()
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		n, err := f.Read(buf)
		if n > 0 {
			sum.Write(buf[:n])
			size += int64(n)
		}
		if err == io.EOF {
			return hex.EncodeToString(sum.Sum(nil)), size, nil
		}
		if err != nil {
			return "", 0, err
		}
	}
}

type scanned struct {
	index int
	rel   string
	size  int64
}

func scanFiles(root string) ([]scanned, int64, error) {
	var out []scanned
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, scanned{rel: filepath.ToSlash(rel), size: info.Size()})
		total += info.Size()
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	for i := range out {
		out[i].index = i
	}
	return out, total, nil
}

type reporter struct {
	mu        sync.Mutex
	processed atomic.Int64
	total     int64
	last      time.Time
	notify    func(Progress)
}

func (r *reporter) advance(bytes int64, current string) {
	processed := r.processed.Add(bytes)
	if r.notify == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if processed < r.total && now.Sub(r.last) < progressPeriod {
		return
	}
	r.last = now
	r.notify(Progress{ProcessedBytes: processed, TotalBytes: r.total, CurrentFile: current})
}

func hashAll(
	ctx context.Context,
	files []scanned,
	total int64,
	onProgress func(Progress),
	work func(context.Context, scanned, []byte) error,
) error {
	rep := &reporter{total: total, notify: onProgress}
	queue := make(chan scanned)
	group, ctx := errgroup.WithContext(ctx)

	for i := 0; i < workerCount(); i++ {
		group.Go(func() error {
			buf := make([]byte, hashBufferSize)
			for item := range queue {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := work(ctx, item, buf); err != nil {
					return err
				}
				rep.advance(item.size, item.rel)
			}
			return nil
		})
	}

	group.Go(func() error {
		defer close(queue)
		for _, item := range files {
			select {
			case queue <- item:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	return group.Wait()
}

// BuildManifest hashes every file below root with a bounded worker pool.
func BuildManifest(ctx context.Context, root string, onProgress func(Progress)) (FileManifest, error) {
	files, total, err := scanFiles(root)
	if err != nil {
		return FileManifest{}, err
	}
	entries := make([]FileManifestEntry, len(files))
	err = hashAll(ctx, files, total, onProgress, func(ctx context.Context, item scanned, buf []byte) error {
		hash, size, err := hashFile(ctx, filepath.Join(root, filepath.FromSlash(item.rel)), buf)
		if err != nil {
			return err
		}
		entries[item.index] = FileManifestEntry{Path: item.rel, Size: size, Hash: hash}
		return nil
	})
	if err != nil {
		return FileManifest{}, err
	}
	return FileManifest{
		Root:      root,
		TotalSize: total,
		Entries:   entries,
		CreatedAt: time.Now(),
	}, nil
}

// statIssue and hashIssue turn a failure into a named result instead of a bare
// error: a file that cannot be read is unread, not damaged, and only a hash
// that was actually computed can disagree with the manifest.
func statIssue(stat fs.FileInfo, err error, entry FileManifestEntry) IssueKind {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return IssueMissing
	case err != nil:
		return IssueUnreadable
	case stat.Size() != entry.Size:
		return IssueSize
	}
	return ""
}

func hashIssue(hash string, err error, entry FileManifestEntry) IssueKind {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return IssueMissing
	case err != nil:
		return IssueUnreadable
	case hash != entry.Hash:
		return IssueCorrupted
	}
	return ""
}

// VerifyManifest re-hashes the files recorded in a manifest and reports what no
// longer matches. Files outside the manifest are listed separately so that user
// content is never mistaken for damage.
func VerifyManifest(ctx context.Context, root string, manifest FileManifest, onProgress func(Progress)) (ManifestResult, error) {
	result := ManifestResult{TotalFiles: len(manifest.Entries)}
	files := make([]scanned, 0, len(manifest.Entries))
	expected := make(map[string]FileManifestEntry, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		expected[entry.Path] = entry
		files = append(files, scanned{rel: entry.Path, size: entry.Size})
		result.TotalBytes += entry.Size
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })

	var mu sync.Mutex
	record := func(issue *ManifestIssue, entry FileManifestEntry) {
		mu.Lock()
		defer mu.Unlock()
		if issue != nil {
			result.Issues = append(result.Issues, *issue)
			return
		}
		result.OkFiles++
		result.OkBytes += entry.Size
	}

	err := hashAll(ctx, files, result.TotalBytes, onProgress, func(ctx context.Context, item scanned, buf []byte) error {
		entry := expected[item.rel]
		path := filepath.Join(root, filepath.FromSlash(item.rel))
		stat, err := os.Stat(path)
		if kind := statIssue(stat, err, entry); kind != "" {
			record(&ManifestIssue{Path: item.rel, Kind: kind}, entry)
			return nil
		}
		hash, _, err := hashFile(ctx, path, buf)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if kind := hashIssue(hash, err, entry); kind != "" {
			record(&ManifestIssue{Path: item.rel, Kind: kind}, entry)
			return nil
		}
		record(nil, entry)
		return nil
	})
	if err != nil {
		return ManifestResult{}, err
	}

	present, _, err := scanFiles(root)
	if err != nil {
		return ManifestResult{}, fmt.Errorf("обход %s: %w", root, err)
	}
	for _, item := range present {
		if _, ok := expected[item.rel]; !ok {
			result.Extra = append(result.Extra, item.rel)
		}
	}
	sort.Slice(result.Issues, func(i, j int) bool { return result.Issues[i].Path < result.Issues[j].Path })
	sort.Strings(result.Extra)
	return result, nil
}

// Managed reports whether a path belongs to the files a manifest owns.
func (m FileManifest) Managed(rel string) bool {
	target := filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	for _, entry := range m.Entries {
		if entry.Path == target {
			return true
		}
	}
	return false
}
