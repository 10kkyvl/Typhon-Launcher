// Command mkdelta builds the binary patch that takes an installed launcher
// from one release to the next. CI runs it once per previous release; the
// patch is verified here by applying it before anything gets near a manifest.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"typhon/internal/selfupdate"
	"typhon/internal/selfupdate/delta"
	"typhon/internal/storage"
)

var (
	errMissingOld = errors.New("mkdelta: -old is required")
	errMissingNew = errors.New("mkdelta: -new is required")
	errMissingOut = errors.New("mkdelta: -out is required")
	errSameBuild  = errors.New("mkdelta: the old and the new build are the same file content")
	errNotWorking = errors.New("mkdelta: the patch does not rebuild the new executable")
	errNotSmaller = errors.New("mkdelta: the patch is not smaller than the new executable")
)

type config struct {
	oldPath string
	newPath string
	out     string
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("mkdelta", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg config
	fs.StringVar(&cfg.oldPath, "old", "", "executable of the release the patch starts from")
	fs.StringVar(&cfg.newPath, "new", "", "executable of the release the patch leads to")
	fs.StringVar(&cfg.out, "out", "", "path to write the patch to")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	switch {
	case cfg.oldPath == "":
		return config{}, errMissingOld
	case cfg.newPath == "":
		return config{}, errMissingNew
	case cfg.out == "":
		return config{}, errMissingOut
	}
	return cfg, nil
}

func readExecutable(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("mkdelta: open %q: %w", path, err)
	}
	data, err := io.ReadAll(io.LimitReader(f, selfupdate.MaxArtifactSize+1))
	if cerr := f.Close(); cerr != nil && err == nil {
		err = fmt.Errorf("mkdelta: close %q: %w", path, cerr)
	}
	if err != nil {
		return nil, fmt.Errorf("mkdelta: read %q: %w", path, err)
	}
	if len(data) > selfupdate.MaxArtifactSize {
		return nil, fmt.Errorf("mkdelta: %q is larger than %d bytes", path, selfupdate.MaxArtifactSize)
	}
	return data, nil
}

func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

type report struct {
	OldSize, NewSize, PatchSize int64
	OldSHA, NewSHA              string
	Encode, Apply               time.Duration
}

func run(ctx context.Context, args []string, stderr io.Writer) (report, error) {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return report{}, err
	}
	oldExe, err := readExecutable(cfg.oldPath)
	if err != nil {
		return report{}, err
	}
	newExe, err := readExecutable(cfg.newPath)
	if err != nil {
		return report{}, err
	}
	if bytes.Equal(oldExe, newExe) {
		return report{}, errSameBuild
	}

	var patch bytes.Buffer
	started := time.Now()
	if err := delta.Encode(&patch, oldExe, newExe); err != nil {
		return report{}, err
	}
	encoded := time.Since(started)

	if int64(patch.Len()) >= int64(len(newExe)) {
		return report{}, errNotSmaller
	}

	var rebuilt bytes.Buffer
	started = time.Now()
	n, err := delta.Apply(ctx, &rebuilt, oldExe, bytes.NewReader(patch.Bytes()), int64(len(newExe)))
	if err != nil {
		return report{}, fmt.Errorf("mkdelta: verify the patch: %w", err)
	}
	applied := time.Since(started)
	if n != int64(len(newExe)) || !bytes.Equal(rebuilt.Bytes(), newExe) {
		return report{}, errNotWorking
	}

	if err := storage.WriteAtomic(cfg.out, patch.Bytes()); err != nil {
		return report{}, fmt.Errorf("mkdelta: write %q: %w", cfg.out, err)
	}
	return report{
		OldSize: int64(len(oldExe)), NewSize: int64(len(newExe)), PatchSize: int64(patch.Len()),
		OldSHA: sum(oldExe), NewSHA: sum(newExe),
		Encode: encoded, Apply: applied,
	}, nil
}

func main() {
	//nolint:forbidigo // main of a standalone cmd: the one place invariant 20 allows a root context
	rep, err := run(context.Background(), os.Args[1:], os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) //nolint:forbidigo // forbidigo's own rule is "exit only from main"; this is func main of a standalone cmd
	}
	if _, err := fmt.Fprintf(os.Stdout, "old %d bytes %s\nnew %d bytes %s\npatch %d bytes (%.1f%% of the new executable)\nencode %s, apply %s\n",
		rep.OldSize, rep.OldSHA, rep.NewSize, rep.NewSHA, rep.PatchSize,
		100*float64(rep.PatchSize)/float64(rep.NewSize), rep.Encode.Round(time.Millisecond), rep.Apply.Round(time.Millisecond)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1) //nolint:forbidigo // forbidigo's own rule is "exit only from main"; this is func main of a standalone cmd
	}
}
