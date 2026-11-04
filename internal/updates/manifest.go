package updates

import (
	"context"
	"path"
	"strings"

	"typhon/internal/hashdir"
)

type Progress = hashdir.Progress

type IssueKind = hashdir.IssueKind

const (
	IssueMissing    = hashdir.IssueMissing
	IssueCorrupted  = hashdir.IssueCorrupted
	IssueSize       = hashdir.IssueSize
	IssueUnreadable = hashdir.IssueUnreadable
)

type ManifestIssue = hashdir.Issue

type ManifestResult = hashdir.Result

// launcherOwnedPrefix marks the files Typhon itself keeps in the root of an
// install directory: the install marker (library.MarkerName), the devmock
// installer marker and in-place update staging. The launcher rewrites them
// on its own schedule (a version bump, ownership or uninstall data), so they
// say nothing about whether the game's files are intact.
const launcherOwnedPrefix = ".typhon-"

func launcherOwned(rel string) bool {
	rel = strings.TrimPrefix(rel, "./")
	return !strings.Contains(rel, "/") && strings.HasPrefix(path.Base(rel), launcherOwnedPrefix)
}

func gameEntries(m FileManifest) FileManifest {
	entries := make([]hashdir.Entry, 0, len(m.Entries))
	var total int64
	for _, entry := range m.Entries {
		if launcherOwned(entry.Path) {
			continue
		}
		entries = append(entries, entry)
		total += entry.Size
	}
	m.Entries = entries
	m.TotalSize = total
	return m
}

// BuildManifest hashes every game file below root with a bounded worker pool.
// Launcher-owned files are left out: the manifest describes the game only.
func BuildManifest(ctx context.Context, root string, onProgress func(Progress)) (FileManifest, error) {
	m, err := hashdir.Build(ctx, root, onProgress)
	if err != nil {
		return FileManifest{}, err
	}
	return gameEntries(m), nil
}

// VerifyManifest re-hashes the files recorded in a manifest and reports what
// no longer matches. Files outside the manifest are listed separately so that
// user content is never mistaken for damage. Launcher-owned files are skipped
// on both sides, which also keeps manifests saved before they were excluded
// from reporting the launcher's own marker rewrite as damage.
func VerifyManifest(ctx context.Context, root string, manifest FileManifest, onProgress func(Progress)) (ManifestResult, error) {
	result, err := hashdir.Verify(ctx, root, gameEntries(manifest), onProgress)
	if err != nil {
		return ManifestResult{}, err
	}
	extra := result.Extra[:0:0]
	for _, rel := range result.Extra {
		if !launcherOwned(rel) {
			extra = append(extra, rel)
		}
	}
	if len(extra) == 0 {
		extra = nil
	}
	result.Extra = extra
	return result, nil
}
