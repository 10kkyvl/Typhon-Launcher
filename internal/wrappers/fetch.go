package wrappers

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"typhon/internal/storage"
)

var errStaleCache = errors.New("cache manifest is unreadable")

const (
	downloadTimeout = 10 * time.Minute
	maxRedirects    = 5
	progressStep    = 256 << 10
	manifestName    = "manifest.json"
	archiveTmp      = "archive.download"
)

type downloader struct {
	client *http.Client
}

func newDownloader(allow func(*url.URL) error) *downloader {
	return &downloader{client: &http.Client{
		Timeout: downloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("%w: too many redirects", ErrDownload)
			}
			return allow(req.URL)
		},
	}}
}

type manifest struct {
	ArchiveSHA string            `json:"archiveSha"`
	Files      map[string]string `json:"files"`
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashFile(p string) (sum string, err error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (d *downloader) fetch(ctx context.Context, rawURL, wantSHA, dst string, progress func(done, total int64)) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDownload, err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDownload, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("%w: close body: %w", ErrDownload, cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrDownload, resp.StatusCode)
	}
	total := max(resp.ContentLength, 0)
	return storage.WriteAtomicStream(dst, 0o600, func(w io.Writer) error {
		h := sha256.New()
		var done, last int64
		buf := make([]byte, 32<<10)
		body := io.LimitReader(resp.Body, maxArchiveBytes+1)
		for {
			n, rerr := body.Read(buf)
			if n > 0 {
				if _, err := w.Write(buf[:n]); err != nil {
					return err
				}
				if _, err := h.Write(buf[:n]); err != nil {
					return err
				}
				done += int64(n)
				if done > maxArchiveBytes {
					return fmt.Errorf("%w: archive larger than %d bytes", ErrDownload, maxArchiveBytes)
				}
				if done-last >= progressStep {
					last = done
					progress(done, total)
				}
			}
			if errors.Is(rerr, io.EOF) {
				break
			}
			if rerr != nil {
				return fmt.Errorf("%w: %w", ErrDownload, rerr)
			}
		}
		progress(done, total)
		if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
			return fmt.Errorf("%w: got %s", ErrArchiveHash, got)
		}
		return nil
	})
}

func validEntryName(n string) error {
	if n == "" || len(n) > 260 || strings.ContainsAny(n, "\\\x00:") || path.IsAbs(n) {
		return fmt.Errorf("%w: entry name %q", ErrBadArchive, n)
	}
	for _, seg := range strings.Split(n, "/") {
		if seg == ".." {
			return fmt.Errorf("%w: entry name %q", ErrBadArchive, n)
		}
	}
	return nil
}

func readEntry(r io.Reader, name string) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxEntryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(b) > maxEntryBytes {
		return nil, fmt.Errorf("%w: entry %s larger than %d bytes", ErrBadArchive, name, maxEntryBytes)
	}
	return b, nil
}

type extractor struct {
	want  map[string]string
	found map[string][]byte
}

func newExtractor(files []archiveFile) *extractor {
	e := &extractor{want: map[string]string{}, found: map[string][]byte{}}
	for _, f := range files {
		e.want[f.From] = f.To
	}
	return e
}

func (e *extractor) take(name string, regular bool, open func() (io.Reader, error)) error {
	if err := validEntryName(name); err != nil {
		return err
	}
	to, ok := e.want[name]
	if !ok {
		return nil
	}
	if !regular {
		return fmt.Errorf("%w: entry %s is not a regular file", ErrBadArchive, name)
	}
	if _, dup := e.found[to]; dup {
		return fmt.Errorf("%w: duplicate entry %s", ErrBadArchive, name)
	}
	r, err := open()
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	b, err := readEntry(r, name)
	if err != nil {
		return err
	}
	e.found[to] = b
	return nil
}

func (e *extractor) finish() (map[string][]byte, error) {
	for from, to := range e.want {
		if _, ok := e.found[to]; !ok {
			return nil, fmt.Errorf("%w: entry %s is missing", ErrBadArchive, from)
		}
	}
	return e.found, nil
}

func extractZip(p string, files []archiveFile) (_ map[string][]byte, err error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadArchive, err)
	}
	defer func() {
		if cerr := zr.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	e := newExtractor(files)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			if err := validEntryName(strings.TrimSuffix(f.Name, "/")); err != nil {
				return nil, err
			}
			continue
		}
		err := e.take(f.Name, f.Mode().IsRegular(), func() (io.Reader, error) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			return rc, nil
		})
		if err != nil {
			return nil, err
		}
	}
	return e.finish()
}

func extractTarGz(p string, files []archiveFile) (_ map[string][]byte, err error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBadArchive, err)
	}
	tr := tar.NewReader(gz)
	e := newExtractor(files)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBadArchive, err)
		}
		if h.Typeflag == tar.TypeDir {
			if err := validEntryName(strings.TrimSuffix(h.Name, "/")); err != nil {
				return nil, err
			}
			continue
		}
		err = e.take(h.Name, h.Typeflag == tar.TypeReg, func() (io.Reader, error) { return tr, nil })
		if err != nil {
			return nil, err
		}
	}
	return e.finish()
}

func extract(p string, r release) (map[string][]byte, error) {
	switch r.Kind {
	case "zip":
		return extractZip(p, r.Files)
	case "tgz":
		return extractTarGz(p, r.Files)
	}
	return nil, fmt.Errorf("%w: kind %q", ErrBadArchive, r.Kind)
}

// A damaged or foreign cache is rebuilt from the pinned archive: the caller
// treats errStaleCache as "not cached".
func cacheValid(dir string, r release) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(dir, manifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read cache manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return false, errStaleCache
	}
	if m.ArchiveSHA != r.SHA256 {
		return false, nil
	}
	for _, f := range r.Files {
		want, ok := m.Files[f.To]
		if !ok {
			return false, nil
		}
		got, err := hashFile(filepath.Join(dir, filepath.FromSlash(f.To)))
		if errors.Is(err, fs.ErrNotExist) || (err == nil && got != want) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("hash cached %s: %w", f.To, err)
		}
	}
	return true, nil
}

func (s *Service) ensure(ctx context.Context, gameID string, r release) (string, error) {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	dir := filepath.Join(s.root, r.Wrapper+"-"+r.Version)
	ok, err := cacheValid(dir, r)
	if err != nil && !errors.Is(err, errStaleCache) {
		return "", err
	}
	if ok {
		return dir, nil
	}
	part := dir + ".partial"
	if err := os.RemoveAll(part); err != nil {
		return "", fmt.Errorf("clear %s: %w", part, err)
	}
	if err := os.MkdirAll(part, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", part, err)
	}
	archive := filepath.Join(part, archiveTmp)
	err = s.dl.fetch(ctx, r.URL, r.SHA256, archive, func(done, total int64) {
		s.emit(ProgressEvent, Progress{GameID: gameID, Stage: "download", Done: done, Total: total})
	})
	if err != nil {
		return "", err
	}
	s.emit(ProgressEvent, Progress{GameID: gameID, Stage: "extract"})
	found, err := extract(archive, r)
	if err != nil {
		return "", err
	}
	m := manifest{ArchiveSHA: r.SHA256, Files: map[string]string{}}
	for to, data := range found {
		p := filepath.Join(part, filepath.FromSlash(to))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return "", fmt.Errorf("create %s: %w", filepath.Dir(p), err)
		}
		if err := storage.WriteAtomic(p, data); err != nil {
			return "", err
		}
		m.Files[to] = hashBytes(data)
	}
	if err := os.Remove(archive); err != nil {
		return "", fmt.Errorf("remove %s: %w", archive, err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("encode manifest: %w", err)
	}
	if err := storage.WriteAtomic(filepath.Join(part, manifestName), raw); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("clear %s: %w", dir, err)
	}
	if err := os.Rename(part, dir); err != nil {
		return "", fmt.Errorf("publish cache %s: %w", dir, err)
	}
	return dir, nil
}
