// Command devrelease runs a local self-update release server for exercising
// the launcher's update cycle end to end under the devmock build: it builds
// (or takes) a launcher artifact, signs a manifest for it with a one-off
// ed25519 key, and serves both over 127.0.0.1 so a devmock launcher can point
// TYPHON_DEVMOCK_MANIFEST_URL and TYPHON_DEVMOCK_RELEASE_PUBKEY at it.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"typhon/internal/selfupdate"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 5 * time.Second
	buildTimeout      = 5 * time.Minute
)

var (
	errMissingVersion = errors.New("devrelease: -version is required")
	errNotRepoRoot    = errors.New("devrelease: go.mod not found in the current directory; run from the repository root, or pass -artifact")
	errBadSeed        = errors.New("devrelease: -seed must be a base64-encoded 32-byte ed25519 seed")
)

type config struct {
	version  string
	artifact string
	port     int
	seed     string
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("devrelease", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg config
	fs.StringVar(&cfg.version, "version", "", "version to publish in the manifest (required)")
	fs.StringVar(&cfg.artifact, "artifact", "", "path to a pre-built launcher artifact (default: build one with -tags devmock)")
	fs.IntVar(&cfg.port, "port", 8099, "port to bind on 127.0.0.1")
	fs.StringVar(&cfg.seed, "seed", "", "base64-encoded ed25519 seed, 32 bytes (default: generate one)")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.version == "" {
		return config{}, errMissingVersion
	}
	return cfg, nil
}

// resolveArtifact returns the path to the launcher binary to serve. Building
// one is the common path during development; -artifact exists so CI or a
// script that already produced a devmock build does not pay for a second one.
func resolveArtifact(ctx context.Context, cfg config, stdout, stderr io.Writer) (path string, cleanup func(), err error) {
	if cfg.artifact != "" {
		abs, err := filepath.Abs(cfg.artifact)
		if err != nil {
			return "", nil, fmt.Errorf("resolve artifact path: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			return "", nil, fmt.Errorf("stat artifact: %w", err)
		}
		return abs, func() {}, nil
	}

	if _, err := os.Stat("go.mod"); err != nil {
		return "", nil, errNotRepoRoot
	}
	tmpDir, err := os.MkdirTemp("", "typhon-devrelease-")
	if err != nil {
		return "", nil, fmt.Errorf("create build dir: %w", err)
	}
	cleanup = func() {
		if rerr := os.RemoveAll(tmpDir); rerr != nil {
			slog.Warn("remove devrelease build dir", "dir", tmpDir, "error", rerr)
		}
	}

	out := filepath.Join(tmpDir, "typhon")
	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	ldflags := "-X typhon/internal/app.Version=" + cfg.version
	//nolint:gosec // G204: fixed argv (go build ...), cfg.version only appears inside a fixed -ldflags template, not as a shell string (invariant 33)
	cmd := exec.CommandContext(buildCtx, "go", "build", "-tags", "devmock", "-ldflags", ldflags, "-o", out, ".")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("build devmock launcher artifact: %w", err)
	}
	return out, cleanup, nil
}

func resolveKeyPair(encodedSeed string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	if encodedSeed == "" {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
		}
		return priv, pub, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encodedSeed)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errBadSeed, err)
	}
	if len(raw) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("%w: got %d bytes, want %d", errBadSeed, len(raw), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(raw)
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return nil, nil, errBadSeed
	}
	return priv, pub, nil
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

func buildSignedManifest(version string, artifactPath string, port int, priv ed25519.PrivateKey) ([]byte, selfupdate.Artifact, error) {
	size, sum, err := hashFile(artifactPath)
	if err != nil {
		return nil, selfupdate.Artifact{}, err
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
		return nil, selfupdate.Artifact{}, fmt.Errorf("build artifact: %w", err)
	}
	m := selfupdate.Manifest{
		Version:     version,
		PublishedAt: time.Now().UTC(),
		Notes:       "Local devrelease build.",
		Artifacts:   []selfupdate.Artifact{art},
	}
	signed, err := selfupdate.SignManifest(m, priv)
	if err != nil {
		return nil, selfupdate.Artifact{}, fmt.Errorf("sign manifest: %w", err)
	}
	return signed, art, nil
}

type releaseServer struct {
	signed       []byte
	version      string
	artifactName string
	artifactPath string
}

func (s *releaseServer) handleManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(s.signed); err != nil {
		slog.Warn("devrelease: write manifest response", "error", err)
	}
}

func (s *releaseServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	name := r.PathValue("name")
	if version != s.version || name != s.artifactName {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(s.artifactPath)
	if err != nil {
		slog.Error("devrelease: open artifact for download", "error", err)
		http.Error(w, "artifact unavailable", http.StatusInternalServerError)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("devrelease: close artifact file", "error", cerr)
		}
	}()

	info, err := f.Stat()
	if err != nil {
		slog.Error("devrelease: stat artifact for download", "error", err)
		http.Error(w, "artifact unavailable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func newMux(s *releaseServer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /launcher/manifest", s.handleManifest)
	mux.HandleFunc("GET /launcher/download/{version}/{name}", s.handleDownload)
	return mux
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	artifactPath, cleanup, err := resolveArtifact(ctx, cfg, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	priv, pub, err := resolveKeyPair(cfg.seed)
	if err != nil {
		return err
	}

	signed, art, err := buildSignedManifest(cfg.version, artifactPath, cfg.port, priv)
	if err != nil {
		return err
	}

	srv := &releaseServer{signed: signed, version: cfg.version, artifactName: art.Name, artifactPath: artifactPath}
	httpServer := &http.Server{
		Handler:           newMux(srv),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.port)))
	if err != nil {
		return fmt.Errorf("listen on 127.0.0.1:%d: %w", cfg.port, err)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.port)
	if _, err := os.Stdout.WriteString("TYPHON_DEVMOCK_MANIFEST_URL=" + baseURL + "\n"); err != nil {
		return fmt.Errorf("print manifest url: %w", err)
	}
	if _, err := os.Stdout.WriteString("TYPHON_DEVMOCK_RELEASE_PUBKEY=" + base64.StdEncoding.EncodeToString(pub) + "\n"); err != nil {
		return fmt.Errorf("print release pubkey: %w", err)
	}
	slog.Info("devrelease: serving", "url", baseURL, "version", cfg.version, "artifact", artifactPath)

	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}
}

func main() {
	//nolint:forbidigo // invariant 20: context.Background is allowed in main; this is func main of a standalone cmd, the same place root main.go is exempted for
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "devrelease:", err)
		os.Exit(1) //nolint:forbidigo // forbidigo's own rule is "exit only from main"; this is func main of a standalone cmd, the same place root main.go is exempted for
	}
}
