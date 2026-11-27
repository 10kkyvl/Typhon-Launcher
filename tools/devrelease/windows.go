package main

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"typhon/internal/selfupdate"
	"typhon/internal/storage"
)

// The Windows chain is for a qa typhon.exe running in portable mode: it
// downloads the zip named next to the installer entry and swaps typhon.exe in
// place. No installer is built, so the installer entry names a small text
// file that says what it is.
const (
	windowsOS   = "windows"
	windowsArch = "amd64"
	sysoName    = "wails_windows_amd64.syso"

	windowsChainNote = "devrelease: windows/amd64 chain: only the portable zip is served; the installer entry names a placeholder text file, not an installer. Run the qa typhon.exe in portable mode (TYPHON_PORTABLE_DIR or portable.txt) with update.ps1 sourced."
)

var errNoFrontend = errors.New("devrelease: frontend/dist holds no index.html, and the launcher embeds it; build the frontend first: wails3 task common:build:frontend")

// goBuildWindows cross-builds typhon.exe with the flags of `wails3 task
// build:qa` (build/windows/Taskfile.yml) and the given version.
func goBuildWindows(stdout, stderr io.Writer) buildFunc {
	return func(ctx context.Context, version, out string) error {
		if _, err := os.Stat("go.mod"); err != nil {
			return errNotRepoRoot
		}
		if err := checkFrontendBuilt(); err != nil {
			return err
		}
		buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
		defer cancel()
		ldflags := "-w -s -H windowsgui -X typhon/internal/app.Version=" + version
		//nolint:gosec // G204: fixed argv (go build ...), version only appears inside a fixed -ldflags template, not as a shell string (invariant 33)
		cmd := exec.CommandContext(buildCtx, "go", "build", "-tags", "qa", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", out, ".")
		cmd.Env = append(os.Environ(), "GOOS="+windowsOS, "GOARCH="+windowsArch, "CGO_ENABLED=0")
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build (windows qa): %w", err)
		}
		return nil
	}
}

func checkFrontendBuilt() error {
	info, err := os.Stat(filepath.Join("frontend", "dist", "index.html"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errNoFrontend
	case err != nil:
		return fmt.Errorf("check the built frontend: %w", err)
	case !info.Mode().IsRegular():
		return errNoFrontend
	}
	return nil
}

// windowsResource puts the icon and the application manifest (common
// controls, DPI awareness) where go build links them, as the Taskfile's
// generate:syso does. The returned func removes only a file this call
// created; without wails3 the builds go without and the output says so.
func windowsResource(ctx context.Context, stdout, stderr io.Writer) (func() error, error) {
	keep := func() error { return nil }
	_, err := os.Stat(sysoName)
	switch {
	case err == nil:
		return keep, say(stdout, "devrelease: the Windows builds use the existing "+sysoName)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("check %s: %w", sysoName, err)
	}
	wails, err := exec.LookPath("wails3")
	if err != nil {
		return keep, say(stdout, "devrelease: wails3 is not on PATH: the Windows builds go without the Windows resource (no icon, no DPI awareness, no common-controls manifest)")
	}
	genCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	//nolint:gosec // G204: fixed argv; wails3 comes from PATH like the Taskfile's own call
	cmd := exec.CommandContext(genCtx, wails, "generate", "syso", "-arch", windowsArch,
		"-icon", "windows/icon.ico", "-manifest", "windows/wails.exe.manifest", "-info", "windows/info.json",
		"-out", filepath.Join("..", sysoName))
	cmd.Dir = "build"
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	drop := func() error {
		if err := os.Remove(sysoName); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", sysoName, err)
		}
		return nil
	}
	if err := cmd.Run(); err != nil {
		return nil, errors.Join(fmt.Errorf("wails3 generate syso: %w", err), drop())
	}
	return drop, nil
}

func say(w io.Writer, line string) error {
	if _, err := io.WriteString(w, line+"\n"); err != nil {
		return fmt.Errorf("print: %w", err)
	}
	return nil
}

// launcherBuild is the build of the mode: a devmock launcher, or a qa
// typhon.exe for the Windows chain.
func launcherBuild(cfg config, stdout, stderr io.Writer) buildFunc {
	if cfg.windows {
		return goBuildWindows(stdout, stderr)
	}
	return goBuild(stdout, stderr)
}

// buildLaunchers resolves the chain's launchers with build and, for a
// Windows build, keeps the generated resource only as long as the builds
// need it.
func buildLaunchers(ctx context.Context, cfg config, versions []string, build buildFunc, stdout, stderr io.Writer) ([]string, func(), error) {
	if !cfg.windows || cfg.artifact != "" {
		return resolveArtifacts(ctx, cfg, versions, build)
	}
	drop, err := windowsResource(ctx, stdout, stderr)
	if err != nil {
		return nil, nil, err
	}
	paths, cleanup, err := resolveArtifacts(ctx, cfg, versions, build)
	if derr := drop(); derr != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, errors.Join(err, derr)
	}
	return paths, cleanup, err
}

func portableZipName(version string) string { return "typhon-" + version + "-portable.zip" }

func placeholderName(version string) string { return "typhon-" + version + "-not-an-installer.txt" }

func downloadURL(port int, version, name string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/launcher/download/%s/%s", port, version, name)
}

func prepareWindowsReleases(versions, exes []string, workDir string, port int, templates []noteTemplate, published time.Time, priv ed25519.PrivateKey) ([]release, error) {
	return prepareChain(versions, exes, templates, published, func(version, exe, summary string, history []selfupdate.ReleaseNote) (release, error) {
		return newWindowsRelease(version, exe, filepath.Join(workDir, version), port, summary, history, published, priv)
	})
}

func newWindowsRelease(version, exe, dir string, port int, summary string, history []selfupdate.ReleaseNote, published time.Time, priv ed25519.PrivateKey) (release, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return release{}, fmt.Errorf("create release dir for %s: %w", version, err)
	}
	zipPath := filepath.Join(dir, portableZipName(version))
	if err := writePortableZip(exe, zipPath); err != nil {
		return release{}, err
	}
	if err := selfupdate.CheckPortableZip(zipPath); err != nil {
		return release{}, fmt.Errorf("the portable launcher would refuse %s: %w", zipPath, err)
	}
	placeholder := filepath.Join(dir, placeholderName(version))
	text := fmt.Sprintf("devrelease publishes no installer for %s.\r\nA portable qa launcher downloads %s instead; this file only fills the installer entry of the manifest.\r\n", version, portableZipName(version))
	if err := storage.WriteAtomic(placeholder, []byte(text)); err != nil {
		return release{}, fmt.Errorf("write the installer placeholder: %w", err)
	}

	size, sum, err := hashFile(placeholder)
	if err != nil {
		return release{}, err
	}
	zipSize, zipSum, err := hashFile(zipPath)
	if err != nil {
		return release{}, err
	}
	art := selfupdate.Artifact{
		OS:             windowsOS,
		Arch:           windowsArch,
		Kind:           selfupdate.KindInstaller,
		Name:           placeholderName(version),
		URL:            downloadURL(port, version, placeholderName(version)),
		Size:           size,
		SHA256:         sum,
		PortableName:   portableZipName(version),
		PortableURL:    downloadURL(port, version, portableZipName(version)),
		PortableSize:   zipSize,
		PortableSHA256: zipSum,
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
	return release{version: version, path: placeholder, portablePath: zipPath, art: art, m: m, signed: signed}, nil
}

// writePortableZip stores exe as the archive's one top-level typhon.exe,
// the shape applyPortableZip takes.
func writePortableZip(exe, zipPath string) (err error) {
	//nolint:gosec // G304: exe is our own build output in a temp dir or the developer's own -artifact flag to this local dev tool
	in, err := os.Open(exe)
	if err != nil {
		return fmt.Errorf("open launcher %q: %w", exe, err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close launcher %q: %w", exe, cerr)
		}
	}()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat launcher %q: %w", exe, err)
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("zip header for %q: %w", exe, err)
	}
	hdr.Name = "typhon.exe"
	hdr.Method = zip.Deflate

	//nolint:gosec // G304: zipPath is inside the tool's own temp dir
	out, err := os.OpenFile(zipPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %q: %w", zipPath, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %q: %w", zipPath, cerr)
		}
	}()
	zw := zip.NewWriter(out)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("add typhon.exe to %q: %w", zipPath, err)
	}
	if _, err := io.Copy(w, in); err != nil {
		return fmt.Errorf("write typhon.exe into %q: %w", zipPath, err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finish %q: %w", zipPath, err)
	}
	return nil
}

// windowsWorkDir holds the zips and placeholders of one run.
func windowsWorkDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", "typhon-devrelease-windows-")
	if err != nil {
		return "", nil, fmt.Errorf("create the Windows release dir: %w", err)
	}
	return dir, func() {
		if rerr := os.RemoveAll(dir); rerr != nil {
			slog.Warn("remove devrelease Windows release dir", "dir", dir, "error", rerr)
		}
	}, nil
}
