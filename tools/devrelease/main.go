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
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	errBadSeconds     = errors.New("devrelease: -seconds must not be negative")
	errBadSeed        = errors.New("devrelease: -seed must be a base64-encoded 32-byte ed25519 seed")
)

type config struct {
	version  string
	artifact string
	port     int
	seed     string
	state    string
	count    int
	notes    string
	seconds  int
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("devrelease", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg config
	fs.StringVar(&cfg.version, "version", "", "version to publish in the manifest (required)")
	fs.StringVar(&cfg.artifact, "artifact", "", "path to a pre-built launcher artifact (default: build one with -tags devmock)")
	fs.IntVar(&cfg.port, "port", 8099, "port to bind on 127.0.0.1")
	fs.StringVar(&cfg.seed, "seed", "", "base64-encoded ed25519 seed, 32 bytes (default: the -state key, else a one-off key)")
	fs.IntVar(&cfg.count, "count", 3, "how many consecutive patch versions to build from -version and offer one after another (1 with -artifact)")
	fs.StringVar(&cfg.notes, "notes", "", "JSON array of {summary, changes:[{kind,text}]} release notes, entry i for version i (default: a built-in sample)")
	fs.IntVar(&cfg.seconds, "seconds", 20, "about how long one whole artifact download takes, so progress, cancel and resume can be watched (0: full speed)")
	fs.StringVar(&cfg.state, "state", "", "directory that keeps the signing key between runs and receives update.env for a wrapper to source")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.version == "" {
		return config{}, errMissingVersion
	}
	if cfg.seconds < 0 {
		return config{}, errBadSeconds
	}
	if cfg.artifact != "" {
		countSet := false
		fs.Visit(func(f *flag.Flag) { countSet = countSet || f.Name == "count" })
		if !countSet {
			cfg.count = 1
		} else if cfg.count != 1 {
			return config{}, errArtifactWithCount
		}
	}
	return cfg, nil
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

// keyPairFor prefers an explicit -seed, then the key kept in -state, and
// only then a one-off key that dies with the process.
func keyPairFor(cfg config) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	if cfg.seed != "" || cfg.state == "" {
		return resolveKeyPair(cfg.seed)
	}
	return stateKeyPair(cfg.state)
}

func closeListener(l net.Listener, cause error) error {
	if err := l.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("close listener: %w", err))
	}
	return cause
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	versions, err := versionChain(cfg.version, cfg.count)
	if err != nil {
		return err
	}
	templates, err := loadNotes(cfg.notes)
	if err != nil {
		return err
	}
	paths, cleanup, err := resolveArtifacts(ctx, cfg, versions, goBuild(stdout, stderr))
	if err != nil {
		return err
	}
	defer cleanup()

	priv, pub, err := keyPairFor(cfg)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.port)))
	if err != nil {
		return fmt.Errorf("listen on 127.0.0.1:%d: %w", cfg.port, err)
	}
	port := cfg.port
	if addr, ok := listener.Addr().(*net.TCPAddr); ok {
		port = addr.Port
	}

	releases, err := prepareReleases(versions, paths, port, templates, time.Now().UTC(), priv)
	if err != nil {
		return closeListener(listener, err)
	}

	srv := newReleaseServer(releases, cfg.seconds, sleepCtx)
	httpServer := &http.Server{
		Handler:           newMux(srv),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	lines := envLines(baseURL, pub)
	for _, line := range lines {
		if _, err := io.WriteString(stdout, line+"\n"); err != nil {
			return closeListener(listener, fmt.Errorf("print env line: %w", err))
		}
	}
	if cfg.state != "" {
		if err := writeEnvFile(cfg.state, lines); err != nil {
			return closeListener(listener, err)
		}
	}
	slog.Info("devrelease: serving", "url", baseURL, "versions", strings.Join(versions, ","))

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
