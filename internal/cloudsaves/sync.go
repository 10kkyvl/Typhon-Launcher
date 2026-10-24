package cloudsaves

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"typhon/internal/account"
	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/uierr"
)

const (
	downloadSlack      = 1 << 20
	transferTimeout    = 15 * time.Minute
	codeRotationFailed = "savebackup.rotation_failed"
	codeBackupRunning  = "savebackup.game_running"
)

type mode int

const (
	modeAuto mode = iota
	modeKeepLocal
	modeKeepRemote
)

type actionKind int

const (
	actSynced actionKind = iota
	actSkip
	actUpload
	actDownload
	actConflict
)

type action struct {
	kind    actionKind
	ifMatch string
	force   bool
	err     error
}

// decide compares three digests: what is on disk now (local), what this device
// last saw agree with the cloud (st), and what the cloud holds now (remote).
// An empty local digest means the saves folder has no files.
func decide(m mode, local string, st gameState, remote account.Save, hasRemote bool) action {
	switch m {
	case modeKeepLocal:
		if local == "" {
			return action{kind: actSkip, err: errEmpty}
		}
		return action{kind: actUpload, force: true}
	case modeKeepRemote:
		if !hasRemote {
			return action{kind: actSkip, err: errRemoteMissing}
		}
		return action{kind: actDownload}
	}
	if !hasRemote {
		if local == "" {
			return action{kind: actSkip, err: errEmpty}
		}
		return action{kind: actUpload}
	}
	if local == "" {
		return action{kind: actDownload}
	}
	if remote.Digest == local {
		return action{kind: actSynced}
	}
	if remote.Digest == st.RemoteDigest {
		if local == st.LocalDigest {
			return action{kind: actSynced}
		}
		return action{kind: actUpload, ifMatch: remote.Digest}
	}
	if local == st.LocalDigest {
		return action{kind: actDownload}
	}
	return action{kind: actConflict}
}

func skippable(err error) bool {
	for _, target := range []error{errSavesMissing, errSavesAmbig, errGameRunning, errEmpty, errRemoteMissing, errExcluded, errDuplicate} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func fail(err error) outcome {
	var api *account.Error
	if errors.As(err, &api) && api.Code == account.CodeSaveBusy {
		return outcome{state: StatePending, err: err}
	}
	if skippable(err) {
		return outcome{state: StateSkipped, err: err}
	}
	return outcome{state: StateFailed, err: err}
}

// resolveRoot picks the one folder both hashing and the restore use. A saves
// folder the player set takes precedence exactly as in savebackup: when it is
// gone the tree is empty (missing) and the restore creates it, instead of
// hashing whatever detection finds elsewhere.
func (s *Service) resolveRoot(ctx context.Context, g library.Game) (root string, missing bool, err error) {
	if g.SavesDir == "" {
		root, err = s.locate(ctx, g.ID)
		return root, false, err
	}
	info, err := os.Stat(g.SavesDir)
	switch {
	case err == nil && info.IsDir():
		return g.SavesDir, false, nil
	case err == nil:
		return "", false, fmt.Errorf("%w: %s", errSavesNotDir, g.SavesDir)
	case errors.Is(err, fs.ErrNotExist):
		return g.SavesDir, true, nil
	}
	return "", false, fmt.Errorf("%w: %w", errSavesUnread, err)
}

func (s *Service) isRunning(id string) bool {
	for _, running := range s.lib.GetRunningGames() {
		if running == id {
			return true
		}
	}
	return false
}

func (s *Service) locate(ctx context.Context, id string) (string, error) {
	res, err := s.lib.LocateSaves(ctx, id)
	if err != nil {
		return "", err
	}
	switch {
	case res.Path != "":
		return res.Path, nil
	case len(res.Candidates) > 0:
		return "", fmt.Errorf("%w: вариантов %d", errSavesAmbig, len(res.Candidates))
	case res.Unreadable > 0:
		return "", fmt.Errorf("%w: каталогов %d", errSavesUnread, res.Unreadable)
	}
	return "", errSavesMissing
}

// treeOf hashes the saves folder, reusing the previous digest while no file
// changed its name, size or modification time.
func (s *Service) treeOf(ctx context.Context, root string) (tree, error) {
	files, err := scanTree(ctx, root)
	if err != nil {
		return tree{}, err
	}
	sig := signature(files)
	if c, ok := s.cache[root]; ok && c.sig == sig {
		t := summarize(files)
		t.digest, t.sig = c.digest, sig
		return t, nil
	}
	t, err := hashFiles(ctx, files)
	if err != nil {
		return tree{}, err
	}
	t.sig = sig
	s.cache[root] = cachedTree{sig: sig, digest: t.digest}
	return t, nil
}

// syncGame brings one game's saves and its cloud copy together. It never
// writes to either side unless the three digests prove the other side did not
// change, and a change on both sides becomes a recorded conflict.
func (s *Service) syncGame(ctx context.Context, r *run, g library.Game, m mode) outcome {
	gameID, key := g.ID, g.CanonicalGameID
	if key == "" {
		return fail(errNoCanonicalID)
	}
	if !account.ValidSaveGameID(key) {
		return fail(errInvalidID)
	}
	st := s.gameState(key)
	if m == modeAuto && st.Excluded {
		return fail(errExcluded)
	}
	if s.isRunning(gameID) {
		return fail(errGameRunning)
	}
	root, missing, err := s.resolveRoot(ctx, g)
	if err != nil {
		return fail(err)
	}
	var t tree
	if !missing {
		if t, err = s.treeOf(ctx, root); err != nil {
			return fail(err)
		}
	}
	remote, hasRemote := r.remote[key]
	act := decide(m, t.digest, st, remote, hasRemote)
	switch act.kind {
	case actSynced:
		return s.markSynced(key, t.digest, remote.Digest, st)
	case actSkip:
		return fail(act.err)
	case actUpload:
		return s.upload(ctx, r, key, t, act)
	case actDownload:
		return s.download(ctx, r, gameID, key, remote)
	}
	return s.conflict(key, t, st, remote)
}

func (s *Service) markSynced(key, local, remote string, st gameState) outcome {
	now := s.now()
	if st.LocalDigest == local && st.RemoteDigest == remote && st.Conflict == nil {
		return outcome{state: StateSynced, syncedAt: now}
	}
	err := s.commit(key, func(g *gameState) {
		g.LocalDigest, g.RemoteDigest, g.SyncedAt, g.Conflict = local, remote, now, nil
	})
	if err != nil {
		return fail(err)
	}
	return outcome{state: StateSynced, syncedAt: now, changed: true}
}

func (s *Service) conflict(key string, t tree, st gameState, remote account.Save) outcome {
	c := &Conflict{
		LocalDigest:     t.digest,
		LocalUpdatedAt:  t.newest,
		LocalSize:       t.size,
		RemoteDigest:    remote.Digest,
		RemoteUpdatedAt: remote.UpdatedAt,
		RemoteSize:      remote.SizeBytes,
		RemoteDevice:    remote.DeviceID,
	}
	same := st.Conflict != nil && st.Conflict.LocalDigest == c.LocalDigest && st.Conflict.RemoteDigest == c.RemoteDigest
	if !same {
		if err := s.commit(key, func(g *gameState) { g.Conflict = c }); err != nil {
			return fail(err)
		}
	}
	return outcome{state: StateConflict, conflict: c, changed: !same}
}

func (s *Service) upload(ctx context.Context, r *run, key string, t tree, act action) outcome {
	if r.list.MaxBytes <= 0 {
		return fail(errUnknownLimit)
	}
	if prev, ok := s.refused[key]; ok && prev.sig == t.sig && prev.limit == r.list.MaxBytes {
		return fail(prev.err)
	}
	if t.size > maxUnpackedBytes {
		return fail(fmt.Errorf("%w: файлы занимают %d байт, допустимо %d", errTooLarge, t.size, int64(maxUnpackedBytes)))
	}
	a, err := s.build(ctx, s.store.tmpDir(), t.files, r.list.MaxBytes)
	if errors.Is(err, errTooLarge) {
		// The same files cannot fit the same limit: remember the refusal until
		// a file or the limit changes, so no pass zips them again.
		s.refused[key] = refusal{sig: t.sig, limit: r.list.MaxBytes, err: err}
	}
	if err != nil {
		return fail(err)
	}
	delete(s.refused, key)
	defer removeTemp(a.path)

	saved, err := s.send(ctx, key, a, act)
	var clash *account.ErrSaveConflict
	if errors.As(err, &clash) && clash.Current == nil && act.ifMatch != "" {
		// The server has no record any more (deleted from another device), so the
		// digest we remembered is void: forget it and upload as a first upload.
		if cerr := s.commit(key, func(g *gameState) { g.RemoteDigest = "" }); cerr != nil {
			return fail(cerr)
		}
		act.ifMatch = ""
		saved, err = s.send(ctx, key, a, act)
		clash = nil
		if errors.As(err, &clash) {
			return fail(errConflictRetry)
		}
	}
	if errors.As(err, &clash) {
		if clash.Current == nil {
			return fail(errConflictRetry)
		}
		return s.conflict(key, t, s.gameState(key), *clash.Current)
	}
	if err != nil {
		return fail(err)
	}
	if saved.Digest != a.digest {
		return fail(errDigest)
	}
	now := s.now()
	if err := s.commit(key, func(g *gameState) {
		g.LocalDigest, g.RemoteDigest, g.SyncedAt, g.Conflict = a.digest, saved.Digest, now, nil
	}); err != nil {
		return fail(err)
	}
	s.mu.Lock()
	s.usage.UsedBytes += saved.SizeBytes - r.remote[key].SizeBytes
	s.mu.Unlock()
	r.remote[key] = saved
	return outcome{state: StateSynced, syncedAt: now, changed: true}
}

func (s *Service) send(ctx context.Context, key string, a archive, act action) (account.Save, error) {
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	f, err := os.Open(a.path)
	if err != nil {
		return account.Save{}, err
	}
	saved, err := s.client.UploadSave(ctx, key, f, a.size, account.SaveUpload{
		Digest:   a.digest,
		DeviceID: s.deviceID(),
		IfMatch:  act.ifMatch,
		Force:    act.force,
	})
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return saved, err
}

func downloadLimit(maxBytes, remoteSize int64) (int64, error) {
	limit := max(maxBytes, remoteSize)
	if limit <= 0 {
		return 0, errUnknownLimit
	}
	limit += downloadSlack
	if limit > maxUnpackedBytes+downloadSlack {
		return 0, fmt.Errorf("%w: архив в облаке больше %d байт", errTooLarge, int64(maxUnpackedBytes))
	}
	return limit, nil
}

func removeTemp(path string) {
	// A leftover temporary file costs disk only and is swept at the next start.
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("remove cloud saves temporary files", "path", path, "error", err)
	}
}

func (s *Service) download(ctx context.Context, r *run, gameID, key string, remote account.Save) outcome {
	limit, err := downloadLimit(r.list.MaxBytes, remote.SizeBytes)
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(s.store.tmpDir(), 0o700); err != nil {
		return fail(err)
	}
	work, err := os.MkdirTemp(s.store.tmpDir(), key+"-")
	if err != nil {
		return fail(err)
	}
	defer removeTemp(work)

	zipPath := filepath.Join(work, "save.zip")
	if err := s.fetch(ctx, key, zipPath, limit, remote); err != nil {
		return fail(err)
	}
	files := filepath.Join(work, "files")
	if err := os.Mkdir(files, 0o700); err != nil {
		return fail(err)
	}
	if err := extractArchive(ctx, zipPath, files); err != nil {
		return fail(err)
	}
	t, err := hashTree(ctx, files)
	if err != nil {
		return fail(err)
	}
	if t.digest != remote.Digest {
		return fail(errDigest)
	}

	snap, err := s.backups.SnapshotPath(ctx, gameID, files, savebackup.KindCloud)
	if err != nil {
		rotationOnly := snap.ID != "" && uierr.Code(err) == codeRotationFailed
		if !rotationOnly {
			return fail(backupErr(err))
		}
	}
	if err := s.backups.Restore(ctx, gameID, snap.ID); err != nil {
		if uierr.Code(err) == codeBackupRunning {
			return fail(errGameRunning)
		}
		return fail(backupErr(err))
	}
	now := s.now()
	if err := s.commit(key, func(g *gameState) {
		g.LocalDigest, g.RemoteDigest, g.SyncedAt, g.Conflict = remote.Digest, remote.Digest, now, nil
	}); err != nil {
		return fail(err)
	}
	return outcome{state: StateSynced, syncedAt: now, changed: true}
}

func (s *Service) fetch(ctx context.Context, key, zipPath string, limit int64, remote account.Save) error {
	ctx, cancel := context.WithTimeout(ctx, transferTimeout)
	defer cancel()
	f, err := os.OpenFile(zipPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	dl, err := s.client.DownloadSave(ctx, key, f, limit)
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	if remote.SizeBytes > 0 && dl.Size != remote.SizeBytes {
		return fmt.Errorf("%w: получено %d байт, в облаке %d", errInvalid, dl.Size, remote.SizeBytes)
	}
	if dl.Digest != "" && dl.Digest != remote.Digest {
		return fmt.Errorf("%w: облачная копия изменилась во время загрузки", errDigest)
	}
	return nil
}

func backupErr(err error) error {
	if uierr.Code(err) != "" {
		return err
	}
	return uierr.Wrap(codeBackup, err)
}

func classify(err error) string {
	if code := uierr.Code(err); code != "" {
		return code
	}
	var clash *account.ErrSaveConflict
	if errors.As(err, &clash) {
		return codeConflictHTTP
	}
	var api *account.Error
	if errors.As(err, &api) {
		switch api.Code {
		case account.CodeSaveQuota:
			return codeQuota
		case account.CodeSaveTooLarge:
			return codeTooLarge
		case account.CodeSaveInvalid:
			return codeInvalid
		case account.CodeSaveNotFound:
			return codeRemoteMissing
		case account.CodeUnauthenticated:
			return codeUnauthorized
		case account.CodeRateLimited:
			return codeRateLimited
		case account.CodeNetwork:
			return codeNetwork
		case account.CodeSaveBusy:
			return codeBusy
		case account.CodeBlocked:
			return codeBlocked
		}
		return codeServer
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return codeCancelled
	}
	return codeFailed
}

func isNotFound(err error) bool {
	var api *account.Error
	return errors.As(err, &api) && api.Code == account.CodeSaveNotFound
}
