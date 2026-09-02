// Command lintbaseline runs golangci-lint over the whole module and compares
// the number of findings with the baseline recorded for this GOOS and build
// tags. The set of compiled files differs per OS, so each platform keeps its
// own number; the number may only go down.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"typhon/internal/storage"
)

var issuesLine = regexp.MustCompile(`(?m)^(\d+) issues`)

var errNoSummary = errors.New("golangci-lint output has no issue summary")

type options struct {
	baseline string
	tags     string
	goos     string
	update   bool
}

func main() {
	if err := run(); err != nil {
		slog.Error("lintbaseline failed", "error", err)
		//nolint:forbidigo // инвариант «завершение процесса только из main»: это и есть main пакета tools/lintbaseline
		os.Exit(1)
	}
}

func run() error {
	var opts options
	flag.StringVar(&opts.baseline, "baseline", ".github/lint-baseline.txt", "baseline file: one `<key> <count>` per line")
	flag.StringVar(&opts.tags, "tags", "", "build tags passed to golangci-lint (--build-tags)")
	flag.StringVar(&opts.goos, "goos", runtime.GOOS, "GOOS the baseline is keyed by")
	flag.BoolVar(&opts.update, "update", false, "record the current count instead of comparing")
	flag.Parse()

	key := opts.goos
	if opts.tags != "" {
		key += "+" + opts.tags
	}

	count, err := countFindings(opts.tags)
	if err != nil {
		return err
	}
	baseline, err := readBaseline(opts.baseline)
	if err != nil {
		return err
	}

	if opts.update {
		baseline[key] = count
		if err := writeBaseline(opts.baseline, baseline); err != nil {
			return err
		}
		slog.Info("lint baseline recorded", "key", key, "count", count, "file", opts.baseline)
		return nil
	}

	want, ok := baseline[key]
	if !ok {
		return fmt.Errorf("no baseline for %q in %s: current count is %d, record it with -update", key, opts.baseline, count)
	}
	switch {
	case count > want:
		return fmt.Errorf("%s: %d findings, baseline is %d: new or changed code added findings", key, count, want)
	case count < want:
		slog.Info("lint findings below baseline, lower it", "key", key, "count", count, "baseline", want, "file", opts.baseline)
	default:
		slog.Info("lint findings match the baseline", "key", key, "count", count)
	}
	return nil
}

func countFindings(tags string) (int, error) {
	args := []string{"run", "./..."}
	if tags != "" {
		args = append(args, "--build-tags", tags)
	}
	cmd := exec.Command("golangci-lint", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return 0, fmt.Errorf("run golangci-lint: %w", err)
	}
	m := issuesLine.FindSubmatch(out.Bytes())
	if m == nil {
		if err == nil {
			return 0, nil
		}
		tail := out.String()
		if len(tail) > 4000 {
			tail = tail[len(tail)-4000:]
		}
		return 0, fmt.Errorf("%w (exit: %w):\n%s", errNoSummary, err, tail)
	}
	return strconv.Atoi(string(m[1]))
}

func readBaseline(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open baseline: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("close baseline", "error", cerr)
		}
	}()
	out := map[string]int{}
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s:%d: want `<key> <count>`, got %q", path, line, text)
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 0 {
			return nil, fmt.Errorf("%s:%d: bad count %q", path, line, fields[1])
		}
		out[fields[0]] = n
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read baseline: %w", err)
	}
	return out, nil
}

func writeBaseline(path string, baseline map[string]int) error {
	keys := make([]string, 0, len(baseline))
	for k := range baseline {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# golangci-lint findings per GOOS[+build tags], measured on a clean checkout.\n")
	b.WriteString("# Checked by `go run ./tools/lintbaseline`; the numbers may only go down.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %d\n", k, baseline[k])
	}
	return storage.WriteAtomic(path, []byte(b.String()))
}
