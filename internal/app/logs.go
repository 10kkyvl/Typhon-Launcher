package app

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"typhon/internal/platform"
	"typhon/internal/settings"
	"typhon/internal/storage"
)

const logFileName = "typhon.log"

var maxExportedLogBytes int64 = 16 << 20

var ErrNoLogs = errors.New("файлы журнала не найдены")

type LogBundle struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	SizeBytes int64  `json:"sizeBytes"`
}

func (s *Service) ExportLogs() (LogBundle, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return LogBundle{}, fmt.Errorf("config dir: %w", err)
	}
	target, err := platform.DownloadsDir()
	if err != nil {
		return LogBundle{}, err
	}
	name := fmt.Sprintf("typhon-logs-%s-%s.zip", Version, time.Now().Format("20060102-150405"))
	bundle, err := writeLogBundle(dir, filepath.Join(target, name), logReport(dir))
	if err != nil {
		slog.Error("export logs", "dir", dir, "error", err)
		return LogBundle{}, err
	}
	slog.Info("export logs", "path", bundle.Path, "bytes", bundle.SizeBytes)
	return bundle, nil
}

func writeLogBundle(dir, path, report string) (LogBundle, error) {
	if dir == "" || path == "" {
		return LogBundle{}, platform.ErrEmptyPath
	}
	names, err := logFiles(dir)
	if err != nil {
		return LogBundle{}, err
	}
	if len(names) == 0 {
		return LogBundle{}, ErrNoLogs
	}
	data, err := buildArchive(dir, report, names)
	if err != nil {
		return LogBundle{}, err
	}
	if err := storage.WriteAtomic(path, data); err != nil {
		return LogBundle{}, err
	}
	return LogBundle{
		Path:      path,
		Name:      filepath.Base(path),
		Dir:       filepath.Dir(path),
		SizeBytes: int64(len(data)),
	}, nil
}

// buildArchive is the single place that assembles the zip both ExportLogs
// writes to disk and BuildLogUpload sends over the network: info.txt plus
// every named log file, in that order. The two callers never diverge on
// what counts as "the bundle" — only on what happens to the bytes after.
func buildArchive(dir, report string, names []string) ([]byte, error) {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	if err := addEntry(archive, "info.txt", strings.NewReader(report)); err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := addLogFile(archive, dir, name); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("close archive: %w", err)
	}
	return buf.Bytes(), nil
}

func gzipBytes(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(raw); err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// BuildLogUpload prepares the manual "send to us" bundle: the same archive
// ExportLogs would write to disk, gzip-compressed for the wire. Reusing
// ExportLogs's own bundle means a user can download and read exactly what a
// send would contain before ever pressing send.
//
// If the compressed result would still exceed maxGzipBytes, the oldest log
// rotations are dropped one at a time — logFiles returns them in ascending
// name order (typhon.log, then .1, .2, ...), so the current typhon.log is
// always index 0 and is never dropped — until it fits or only the current
// log is left. Dropped names are returned so the caller can tell the user
// exactly what did not make it; nothing here is silently truncated.
func BuildLogUpload(maxGzipBytes int64) (data []byte, dropped []string, err error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, nil, fmt.Errorf("config dir: %w", err)
	}
	return buildUploadBundle(dir, logReport(dir), maxGzipBytes)
}

func buildUploadBundle(dir, report string, maxGzipBytes int64) (data []byte, dropped []string, err error) {
	names, err := logFiles(dir)
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, ErrNoLogs
	}
	kept := append([]string(nil), names...)
	for {
		archive, err := buildArchive(dir, report, kept)
		if err != nil {
			return nil, nil, err
		}
		gz, err := gzipBytes(archive)
		if err != nil {
			return nil, nil, err
		}
		if int64(len(gz)) <= maxGzipBytes || len(kept) <= 1 {
			return gz, dropped, nil
		}
		dropped = append(dropped, kept[len(kept)-1])
		kept = kept[:len(kept)-1]
	}
}

func logFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == logFileName || strings.HasPrefix(name, logFileName+".") {
			names = append(names, name)
		}
	}
	return names, nil
}

func addLogFile(archive *zip.Writer, dir, name string) error {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close log file", "name", name, "error", err)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	if info.Size() > maxExportedLogBytes {
		if _, err := f.Seek(info.Size()-maxExportedLogBytes, io.SeekStart); err != nil {
			return fmt.Errorf("seek %s: %w", name, err)
		}
	}
	return addEntry(archive, name, f)
}

func addEntry(archive *zip.Writer, name string, src io.Reader) error {
	w, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("create entry %s: %w", name, err)
	}
	if _, err := io.Copy(w, src); err != nil {
		return fmt.Errorf("write entry %s: %w", name, err)
	}
	return nil
}

func logReport(dir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Typhon %s\n", Version)
	fmt.Fprintf(&b, "Собрано: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "Платформа: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "Каталог данных: %s\n", dir)
	info, err := platform.GetSystemInfo()
	if err != nil {
		fmt.Fprintf(&b, "Система: не определена: %v\n", err)
		return b.String()
	}
	fmt.Fprintf(&b, "Система: %s\n", info.OS)
	fmt.Fprintf(&b, "Процессор: %s, потоков: %d\n", info.CPU, info.Cores)
	fmt.Fprintf(&b, "Память: %d МБ\n", info.RAMBytes/(1<<20))
	return b.String()
}
