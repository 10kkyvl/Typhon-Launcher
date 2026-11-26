package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"typhon/internal/selfupdate"
)

const maxCount = 10

var (
	errBadCount              = fmt.Errorf("devrelease: -count must be between 1 and %d", maxCount)
	errChainNeedsPlainSemver = errors.New("devrelease: -count above 1 needs a MAJOR.MINOR.PATCH -version to count up from")
	errArtifactWithCount     = errors.New("devrelease: -artifact is one build of one version; pass -count 1 with it, or let devrelease build the chain")
)

// release is one version the source can offer: the launcher binary built
// with that version linked in, and the manifest signed for it. On the
// Windows chain path is the installer placeholder and portablePath the zip.
type release struct {
	version      string
	path         string
	portablePath string
	art          selfupdate.Artifact
	m            selfupdate.Manifest
	signed       []byte
}

// buildFunc produces a devmock launcher that reports version as its own.
type buildFunc func(ctx context.Context, version, out string) error

// versionChain counts patch versions up from first. Only the link step
// differs between them, which is all a chain of updates needs.
func versionChain(first string, count int) ([]string, error) {
	if count < 1 || count > maxCount {
		return nil, errBadCount
	}
	if count == 1 {
		return []string{first}, nil
	}
	parts := strings.Split(first, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || len(parts) != 3 {
			return nil, fmt.Errorf("%w: got %q", errChainNeedsPlainSemver, first)
		}
		nums[i] = n
	}
	versions := make([]string, count)
	for i := range versions {
		versions[i] = fmt.Sprintf("%d.%d.%d", nums[0], nums[1], nums[2]+i)
	}
	return versions, nil
}

// resolveArtifacts returns one launcher binary per version. Building them is
// the common path during development; -artifact exists so a script that
// already produced a devmock build does not pay for a second one.
func resolveArtifacts(ctx context.Context, cfg config, versions []string, build buildFunc) (paths []string, cleanup func(), err error) {
	if cfg.artifact != "" {
		if len(versions) != 1 {
			return nil, nil, errArtifactWithCount
		}
		abs, err := filepath.Abs(cfg.artifact)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve artifact path: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			return nil, nil, fmt.Errorf("stat artifact: %w", err)
		}
		return []string{abs}, func() {}, nil
	}

	tmpDir, err := os.MkdirTemp("", "typhon-devrelease-")
	if err != nil {
		return nil, nil, fmt.Errorf("create build dir: %w", err)
	}
	cleanup = func() {
		if rerr := os.RemoveAll(tmpDir); rerr != nil {
			slog.Warn("remove devrelease build dir", "dir", tmpDir, "error", rerr)
		}
	}
	for _, v := range versions {
		out := filepath.Join(tmpDir, v, "typhon")
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("create build dir for %s: %w", v, err)
		}
		if err := build(ctx, v, out); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("build launcher %s: %w", v, err)
		}
		paths = append(paths, out)
	}
	return paths, cleanup, nil
}

// goBuild links the launcher in the current directory with the devmock tag
// and the given version, the same way the test app is built.
func goBuild(stdout, stderr io.Writer) buildFunc {
	return func(ctx context.Context, version, out string) error {
		if _, err := os.Stat("go.mod"); err != nil {
			return errNotRepoRoot
		}
		buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
		defer cancel()
		ldflags := "-X typhon/internal/app.Version=" + version
		//nolint:gosec // G204: fixed argv (go build ...), version only appears inside a fixed -ldflags template, not as a shell string (invariant 33)
		cmd := exec.CommandContext(buildCtx, "go", "build", "-tags", "devmock", "-ldflags", ldflags, "-o", out, ".")
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build: %w", err)
		}
		return nil
	}
}

func hashFile(path string) (size int64, sum string, err error) {
	//nolint:gosec // G703: path is either our own build output in a temp dir or the developer's own -artifact flag to this local dev tool, not external input
	f, err := os.Open(path)
	if err != nil {
		return 0, "", fmt.Errorf("open artifact %q: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close artifact %q: %w", path, cerr)
		}
	}()

	h := sha256.New()
	size, err = io.Copy(h, f)
	if err != nil {
		return 0, "", fmt.Errorf("hash artifact %q: %w", path, err)
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

func artifactName() string {
	return fmt.Sprintf("typhon-%s-%s", runtime.GOOS, runtime.GOARCH)
}

// prepareReleases signs one manifest per version. Each carries the notes of
// its own version and every earlier one in the chain, newest first, the way
// the release manifest carries the CHANGELOG, so a launcher that skips a
// version still gets the history of the ones it skipped.
func prepareReleases(versions, paths []string, port int, templates []noteTemplate, published time.Time, priv ed25519.PrivateKey) ([]release, error) {
	return prepareChain(versions, paths, templates, published, func(version, path, summary string, history []selfupdate.ReleaseNote) (release, error) {
		return newRelease(version, path, port, summary, history, published, priv)
	})
}

// releaseFunc signs the release of one version from its built launcher.
type releaseFunc func(version, path, summary string, history []selfupdate.ReleaseNote) (release, error)

func prepareChain(versions, paths []string, templates []noteTemplate, published time.Time, newRel releaseFunc) ([]release, error) {
	if len(versions) != len(paths) {
		return nil, fmt.Errorf("devrelease: %d versions for %d artifacts", len(versions), len(paths))
	}
	notes := make([]selfupdate.ReleaseNote, len(versions))
	for i, v := range versions {
		notes[i] = templateFor(templates, i).note(v, published)
	}
	releases := make([]release, len(versions))
	for i, v := range versions {
		history := make([]selfupdate.ReleaseNote, 0, i+1)
		for j := i; j >= 0; j-- {
			history = append(history, notes[j])
		}
		rel, err := newRel(v, paths[i], templateFor(templates, i).Summary, history)
		if err != nil {
			return nil, err
		}
		releases[i] = rel
	}
	return releases, nil
}

func newRelease(version, artifactPath string, port int, summary string, history []selfupdate.ReleaseNote, published time.Time, priv ed25519.PrivateKey) (release, error) {
	size, sum, err := hashFile(artifactPath)
	if err != nil {
		return release{}, err
	}
	name := artifactName()
	art := selfupdate.Artifact{
		OS:     runtime.GOOS,
		Arch:   runtime.GOARCH,
		Kind:   selfupdate.KindInstaller,
		Name:   name,
		URL:    fmt.Sprintf("http://127.0.0.1:%d/launcher/download/%s/%s", port, version, name),
		Size:   size,
		SHA256: sum,
	}
	if err := art.Validate(); err != nil {
		return release{}, fmt.Errorf("build artifact: %w", err)
	}
	m := selfupdate.Manifest{
		Version:     version,
		PublishedAt: published,
		Notes:       summary,
		Releases:    history,
		Artifacts:   []selfupdate.Artifact{art},
	}
	if err := m.Validate(); err != nil {
		return release{}, fmt.Errorf("manifest for %s: %w", version, err)
	}
	signed, err := selfupdate.SignManifest(m, priv)
	if err != nil {
		return release{}, fmt.Errorf("sign manifest: %w", err)
	}
	return release{version: version, path: artifactPath, art: art, m: m, signed: signed}, nil
}
