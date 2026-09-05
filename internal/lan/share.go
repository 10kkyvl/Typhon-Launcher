package lan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"typhon/internal/uierr"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

var (
	errSymlinkInShare   = uierr.New("lan.share_symlink", "lan: shared tree contains a symlink")
	errIrregularInShare = uierr.New("lan.share_irregular_file", "lan: shared tree contains a non-regular file")
	errEmptyShareRoot   = uierr.New("lan.share_root_empty", "lan: share root is empty")
	errShareRootNotDir  = uierr.New("lan.share_root_not_dir", "lan: share root is not a directory")
)

// BuildInfo walks root and produces the metainfo.Info that describes it,
// without anything a public torrent would carry (trackers, private flag are
// added by the caller). It never treats a symlink or other non-regular file
// as something safe to skip: silently omitting it here would raise the
// wrong result on the receiving end (invariant 11).
func BuildInfo(ctx context.Context, root string, onProgress func(Progress)) (*metainfo.Info, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errEmptyShareRoot
	}
	root = filepath.Clean(root)
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}
	if !rootInfo.Mode().IsDir() {
		return nil, errShareRootNotDir
	}

	files, total, err := walkShareFiles(ctx, root)
	if err != nil {
		return nil, err
	}

	info := &metainfo.Info{
		Name:        infoName(root),
		Files:       files,
		PieceLength: metainfo.ChoosePieceLength(total),
	}

	if err := hashInfo(ctx, info, root, total, onProgress); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("hash %s: %w", root, err)
	}
	if onProgress != nil {
		onProgress(Progress{ProcessedBytes: total, TotalBytes: total})
	}
	return info, nil
}

func walkShareFiles(ctx context.Context, root string) ([]metainfo.FileInfo, int64, error) {
	var files []metainfo.FileInfo
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", errSymlinkInShare, path)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%w: %s", errIrregularInShare, path)
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, metainfo.FileInfo{
			Path:   strings.Split(rel, string(filepath.Separator)),
			Length: fi.Size(),
		})
		total += fi.Size()
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(files, func(i, j int) bool {
		return strings.Join(files[i].Path, "/") < strings.Join(files[j].Path, "/")
	})
	return files, total, nil
}

func infoName(root string) string {
	switch b := filepath.Base(root); b {
	case ".", "..", string(filepath.Separator):
		return metainfo.NoName
	default:
		return b
	}
}

// hashInfo fills info.Pieces via metainfo.Info.GeneratePieces, using a
// ctx-aware, progress-reporting reader instead of writing our own SHA-1
// hasher: GeneratePieces already does the piece-boundary bookkeeping, and
// getting that boundary wrong is what makes a torrent silently unshareable.
func hashInfo(ctx context.Context, info *metainfo.Info, root string, total int64, onProgress func(Progress)) error {
	var (
		mu        sync.Mutex
		processed int64
	)
	open := func(fi metainfo.FileInfo) (io.ReadCloser, error) {
		p := filepath.Join(append([]string{root}, fi.Path...)...)
		f, err := os.Open(filepath.Clean(p))
		if err != nil {
			return nil, err
		}
		return &progressReader{
			ReadCloser: f,
			ctx:        ctx,
			name:       strings.Join(fi.Path, "/"),
			total:      total,
			mu:         &mu,
			processed:  &processed,
			onProgress: onProgress,
		}, nil
	}
	return info.GeneratePieces(open)
}

type progressReader struct {
	io.ReadCloser
	ctx        context.Context
	name       string
	total      int64
	mu         *sync.Mutex
	processed  *int64
	onProgress func(Progress)
}

func (r *progressReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.mu.Lock()
		*r.processed += int64(n)
		processed := *r.processed
		r.mu.Unlock()
		if r.onProgress != nil {
			r.onProgress(Progress{ProcessedBytes: processed, TotalBytes: r.total, CurrentFile: r.name})
		}
	}
	return n, err
}

// fingerprint hashes the sorted "rel|size|mtimeUnixNano" of every regular
// file under root. It is cheap next to a full content hash and is what lets
// Share skip re-hashing a tree that has not changed since it was last built.
func fingerprint(ctx context.Context, root string) (string, error) {
	type entry struct {
		rel     string
		size    int64
		modNano int64
	}
	var entries []entry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root || d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", errSymlinkInShare, path)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%w: %s", errIrregularInShare, path)
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entries = append(entries, entry{
			rel:     filepath.ToSlash(rel),
			size:    fi.Size(),
			modNano: fi.ModTime().UnixNano(),
		})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	h := sha256.New()
	for _, e := range entries {
		if _, err := fmt.Fprintf(h, "%s|%d|%d\n", e.rel, e.size, e.modNano); err != nil {
			return "", fmt.Errorf("hash fingerprint entry: %w", err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// buildTorrent bencodes info with the private flag declared (see
// internal/lan client.go for why the flag is a declaration of intent, not a
// relied-upon protection) and returns both the .torrent bytes to persist and
// the resulting v1 infohash.
func buildTorrent(info metainfo.Info) (infoHashHex string, torrentBytes []byte, err error) {
	private := true
	info.Private = &private
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		return "", nil, fmt.Errorf("marshal info: %w", err)
	}
	mi := &metainfo.MetaInfo{
		InfoBytes:    infoBytes,
		CreationDate: time.Now().Unix(),
		CreatedBy:    "typhon-lan",
	}
	var buf bytes.Buffer
	if err := mi.Write(&buf); err != nil {
		return "", nil, fmt.Errorf("encode torrent: %w", err)
	}
	return mi.HashInfoBytes().HexString(), buf.Bytes(), nil
}
