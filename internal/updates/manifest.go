package updates

import (
	"context"

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

// BuildManifest hashes every file below root with a bounded worker pool.
func BuildManifest(ctx context.Context, root string, onProgress func(Progress)) (FileManifest, error) {
	return hashdir.Build(ctx, root, onProgress)
}

// VerifyManifest re-hashes the files recorded in a manifest and reports what
// no longer matches. Files outside the manifest are listed separately so that
// user content is never mistaken for damage.
func VerifyManifest(ctx context.Context, root string, manifest FileManifest, onProgress func(Progress)) (ManifestResult, error) {
	return hashdir.Verify(ctx, root, manifest, onProgress)
}
