package cloudsaves

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"typhon/internal/account"
	"typhon/internal/library"
	"typhon/internal/savebackup"
	"typhon/internal/uierr"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	baseFiles   = map[string]string{"slot1.sav": "one", "profile/slot2.sav": "two"}
	changedBase = map[string]string{"slot1.sav": "one, played further", "profile/slot2.sav": "two"}
	otherDevice = map[string]string{"slot1.sav": "from the other device", "profile/slot2.sav": "two"}
)

func (h *harness) open() {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.svc.mu.Lock()
	h.svc.ctx, h.svc.cancel = ctx, cancel
	h.svc.mu.Unlock()
	h.t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			h.t.Errorf("shutdown: %v", err)
		}
	})
}

func (h *harness) seedRemote(files map[string]string, device string) string {
	h.t.Helper()
	digest := contractDigest(files)
	h.client.seed(gameCloud, device, zipOf(h.t, files), digest, time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC))
	return digest
}

func codeOfErr(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	return uierr.Code(err)
}

func TestDecide(t *testing.T) {
	const (
		d1 = "d1"
		d2 = "d2"
		d3 = "d3"
	)
	remote := func(digest string) account.Save { return account.Save{Digest: digest} }
	cases := []struct {
		name      string
		local     string
		st        gameState
		remote    account.Save
		hasRemote bool
		want      actionKind
		ifMatch   string
		wantErr   error
	}{
		{name: "a: no remote, local present", local: d1, want: actUpload},
		{name: "a: no remote, local present, old record", local: d1, st: gameState{LocalDigest: d2, RemoteDigest: d2}, want: actUpload},
		{name: "a: no remote, nothing local", want: actSkip, wantErr: errEmpty},
		{name: "b: server unchanged, local changed", local: d2, st: gameState{LocalDigest: d1, RemoteDigest: d1}, remote: remote(d1), hasRemote: true, want: actUpload, ifMatch: d1},
		{name: "c: server changed, local unchanged", local: d1, st: gameState{LocalDigest: d1, RemoteDigest: d1}, remote: remote(d3), hasRemote: true, want: actDownload},
		{name: "c: first run with an empty folder", st: gameState{}, remote: remote(d3), hasRemote: true, want: actDownload},
		{name: "d: both changed", local: d2, st: gameState{LocalDigest: d1, RemoteDigest: d1}, remote: remote(d3), hasRemote: true, want: actConflict},
		{name: "d: first run with files on both sides", local: d2, remote: remote(d3), hasRemote: true, want: actConflict},
		{name: "e: digests equal", local: d1, st: gameState{LocalDigest: d2, RemoteDigest: d3}, remote: remote(d1), hasRemote: true, want: actSynced},
		{name: "nothing changed", local: d1, st: gameState{LocalDigest: d1, RemoteDigest: d1}, remote: remote(d1), hasRemote: true, want: actSynced},
		{name: "empty folder, server unchanged: restore, never upload the emptiness", st: gameState{LocalDigest: d1, RemoteDigest: d1}, remote: remote(d1), hasRemote: true, want: actDownload},
		{name: "empty folder, server changed", st: gameState{LocalDigest: d1, RemoteDigest: d2}, remote: remote(d3), hasRemote: true, want: actDownload},
		{name: "empty folder, no record at all", remote: remote(d3), hasRemote: true, want: actDownload},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decide(modeAuto, tc.local, tc.st, tc.remote, tc.hasRemote)
			if got.kind != tc.want || got.ifMatch != tc.ifMatch || got.force {
				t.Fatalf("decide = %+v, want kind %d ifMatch %q", got, tc.want, tc.ifMatch)
			}
			if tc.wantErr != nil && !errors.Is(got.err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", got.err, tc.wantErr)
			}
		})
	}
	t.Run("keep local forces", func(t *testing.T) {
		got := decide(modeKeepLocal, d1, gameState{}, remote(d3), true)
		if got.kind != actUpload || !got.force || got.ifMatch != "" {
			t.Fatalf("decide = %+v", got)
		}
	})
	t.Run("keep local with nothing local", func(t *testing.T) {
		if got := decide(modeKeepLocal, "", gameState{}, remote(d3), true); got.kind != actSkip {
			t.Fatalf("decide = %+v", got)
		}
	})
	t.Run("keep remote without a remote", func(t *testing.T) {
		if got := decide(modeKeepRemote, d1, gameState{}, account.Save{}, false); got.kind != actSkip || !errors.Is(got.err, errRemoteMissing) {
			t.Fatalf("decide = %+v", got)
		}
	})
}

func TestPassUploadsAGameThatIsNotInTheCloud(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)

	h.pass()

	if len(h.client.uploads) != 1 {
		t.Fatalf("%d uploads, want 1", len(h.client.uploads))
	}
	up := h.client.uploads[0]
	if up.key != gameCloud || up.ifMatch != "" || up.force {
		t.Fatalf("upload = key %q ifMatch %q force %v, want a plain first upload", up.key, up.ifMatch, up.force)
	}
	want := contractDigest(baseFiles)
	if up.digest != want {
		t.Fatalf("digest = %s, want %s", up.digest, want)
	}
	if up.device == "" || len(up.device) > 64 {
		t.Fatalf("device id %q does not fit the contract", up.device)
	}
	if got := unzip(t, up.zip); !mapsEqual(got, baseFiles) {
		t.Fatalf("archive has %v, want %v", got, baseFiles)
	}
	st := h.recorded(gameCloud)
	if st.LocalDigest != want || st.RemoteDigest != want || st.SyncedAt.IsZero() || st.Conflict != nil {
		t.Fatalf("recorded state = %+v", st)
	}
	if got := h.status(gameLocal); got.State != StateSynced {
		t.Fatalf("status = %+v", got)
	}
	if evs := h.drain(); len(evs) != 1 || evs[0].GameID != gameLocal || evs[0].Status != StateSynced || evs[0].QuotaBytes == 0 {
		t.Fatalf("events = %+v", evs)
	}
	reloaded, err := h.svc.store.load(ownerID)
	if err != nil || reloaded.Games[gameCloud].RemoteDigest != want {
		t.Fatalf("state file = %+v, err %v", reloaded, err)
	}
}

func TestPassUploadsLocalChangesAgainstTheDigestItLastSaw(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	first := contractDigest(baseFiles)
	writeTree(t, h.saves(), changedBase)

	h.pass()

	if len(h.client.uploads) != 2 {
		t.Fatalf("%d uploads, want 2", len(h.client.uploads))
	}
	up := h.client.uploads[1]
	if up.ifMatch != first || up.force {
		t.Fatalf("second upload ifMatch %q force %v, want If-Match %s", up.ifMatch, up.force, first)
	}
	if got := h.recorded(gameCloud); got.LocalDigest != contractDigest(changedBase) || got.RemoteDigest != contractDigest(changedBase) {
		t.Fatalf("recorded state = %+v", got)
	}
}

func TestPassDownloadsWhatAnotherDeviceUploaded(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	if len(h.client.uploads) != 1 {
		t.Fatalf("setup: %d uploads", len(h.client.uploads))
	}
	remoteDigest := h.seedRemote(otherDevice, "other-device")
	h.drain()

	h.pass()

	if len(h.client.uploads) != 1 {
		t.Fatalf("a download pass uploaded: %d uploads", len(h.client.uploads))
	}
	if h.client.downloads != 1 {
		t.Fatalf("%d downloads, want 1", h.client.downloads)
	}
	if len(h.backups.snapshots) != 1 || h.backups.snapshots[0].kind != savebackup.KindCloud || h.backups.snapshots[0].gameID != gameLocal {
		t.Fatalf("snapshots = %+v, want one cloud snapshot of %s", h.backups.snapshots, gameLocal)
	}
	if !mapsEqual(h.backups.snapshots[0].files, otherDevice) {
		t.Fatalf("snapshot content = %v, want the downloaded files", h.backups.snapshots[0].files)
	}
	if len(h.backups.restores) != 1 {
		t.Fatalf("restores = %v, want 1", h.backups.restores)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v, want %v", got, otherDevice)
	}
	st := h.recorded(gameCloud)
	if st.LocalDigest != remoteDigest || st.RemoteDigest != remoteDigest || st.Conflict != nil {
		t.Fatalf("recorded state = %+v, want both digests %s", st, remoteDigest)
	}
	entries, err := os.ReadDir(h.svc.store.tmpDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary directory has %v (err %v), want it empty", entries, err)
	}
}

func TestPassDownloadsIntoAnEmptySavesFolderOnANewDevice(t *testing.T) {
	h := newHarness(t)
	h.seedRemote(baseFiles, "other-device")

	h.pass()

	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves = %v, want %v", got, baseFiles)
	}
	if len(h.client.uploads) != 0 {
		t.Fatalf("new device uploaded %d times before it had anything", len(h.client.uploads))
	}
}

func TestPassRecordsAConflictAndTouchesNothing(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	remoteDigest := h.seedRemote(otherDevice, "other-device")
	h.drain()
	uploadsBefore := len(h.client.uploads)

	h.pass()

	if len(h.client.uploads) != uploadsBefore || h.client.downloads != 0 || len(h.backups.restores) != 0 {
		t.Fatalf("a conflict wrote: uploads %d downloads %d restores %d", len(h.client.uploads)-uploadsBefore, h.client.downloads, len(h.backups.restores))
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, changedBase) {
		t.Fatalf("local saves changed to %v", got)
	}
	if rs, _ := h.client.remoteOf(gameCloud); rs.save.Digest != remoteDigest {
		t.Fatalf("cloud copy changed to %s", rs.save.Digest)
	}
	c := h.recorded(gameCloud).Conflict
	if c == nil || c.LocalDigest != contractDigest(changedBase) || c.RemoteDigest != remoteDigest || c.RemoteDevice != "other-device" || c.LocalSize == 0 || c.RemoteSize == 0 {
		t.Fatalf("conflict = %+v", c)
	}
	evs := h.drain()
	if len(evs) != 1 || evs[0].Status != StateConflict || evs[0].GameID != gameLocal {
		t.Fatalf("events = %+v, want one conflict event", evs)
	}

	h.pass()
	if evs := h.drain(); len(evs) != 0 {
		t.Fatalf("an unchanged conflict was announced again: %+v", evs)
	}
	if got := h.status(gameLocal); got.State != StateConflict || got.Conflict == nil {
		t.Fatalf("status = %+v", got)
	}
}

func TestPassOnlyRefreshesWhenDigestsAreEqual(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.seedRemote(baseFiles, "other-device")

	h.pass()

	if h.client.downloads != 0 || len(h.client.uploads) != 0 {
		t.Fatalf("equal digests moved data: downloads %d uploads %d", h.client.downloads, len(h.client.uploads))
	}
	want := contractDigest(baseFiles)
	st := h.recorded(gameCloud)
	if st.LocalDigest != want || st.RemoteDigest != want || st.SyncedAt.IsZero() {
		t.Fatalf("recorded state = %+v", st)
	}
	if got := h.status(gameLocal); got.State != StateSynced || got.SyncedAt.IsZero() {
		t.Fatalf("status = %+v", got)
	}
}

func TestUploadConflictFrom409IsRecorded(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	remoteDigest := ""
	h.client.beforeSend = func() {
		remoteDigest = contractDigest(otherDevice)
		h.client.seed(gameCloud, "other-device", zipOf(t, otherDevice), remoteDigest, time.Date(2026, 10, 14, 11, 0, 0, 0, time.UTC))
	}
	h.drain()

	h.pass()

	c := h.recorded(gameCloud).Conflict
	if c == nil || c.RemoteDigest != remoteDigest || c.RemoteDevice != "other-device" {
		t.Fatalf("conflict = %+v, want the digest and device the 409 carried", c)
	}
	if got := h.status(gameLocal); got.State != StateConflict {
		t.Fatalf("status = %+v", got)
	}
	if got := h.recorded(gameCloud); got.RemoteDigest != contractDigest(baseFiles) {
		t.Fatalf("a 409 moved the recorded remote digest to %s", got.RemoteDigest)
	}
}

func TestBrokenStateMakesTheServiceReadOnly(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	path := h.svc.store.path(ownerID)
	broken := []byte(`{"version":1,"data":{"deviceId":`)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}

	h.pass()

	if h.client.calls() != 0 {
		t.Fatalf("%d client calls on an unreadable state, want none", h.client.calls())
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(broken) {
		t.Fatalf("state file was rewritten: %q (err %v)", got, err)
	}
	evs := h.drain()
	if len(evs) != 1 || evs[0].Status != StateFailed || evs[0].Code != codeStateBroken || evs[0].GameID != "" {
		t.Fatalf("events = %+v, want one failed %s", evs, codeStateBroken)
	}
	if st := h.svc.Status(); st.Error != codeStateBroken {
		t.Fatalf("status error = %q", st.Error)
	}

	h.pass()
	if evs := h.drain(); len(evs) != 0 {
		t.Fatalf("the same failure was announced again: %+v", evs)
	}

	h.open()
	if err := h.svc.SyncNow(gameLocal); codeOfErr(t, err) != codeStateBroken {
		t.Fatalf("SyncNow err = %v, want %s", err, codeStateBroken)
	}
	if err := h.svc.DeleteRemote(gameLocal); err == nil {
		t.Fatal("DeleteRemote ran with an unreadable state")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	h.pass()
	if len(h.client.uploads) != 1 {
		t.Fatalf("after the file was removed the pass did not recover: %d uploads", len(h.client.uploads))
	}
}

func TestMissingStateFileStartsEmpty(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	if len(h.client.uploads) != 1 {
		t.Fatalf("a missing state file was treated as an error: %d uploads", len(h.client.uploads))
	}
}

func TestFailedPersistRollsMemoryBackAndRecovers(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	realWrite := h.svc.store.write
	h.svc.store.write = func(string, int, any) error { return errors.New("disk full") }

	h.pass()

	if st := h.recorded(gameCloud); st != (gameState{}) {
		t.Fatalf("memory kept %+v after the write failed", st)
	}
	if got := h.status(gameLocal); got.State != StateFailed || !strings.Contains(got.Message, "disk full") {
		t.Fatalf("status = %+v, want the write error surfaced", got)
	}

	h.svc.store.write = realWrite
	h.pass()
	want := contractDigest(baseFiles)
	if st := h.recorded(gameCloud); st.LocalDigest != want || st.RemoteDigest != want {
		t.Fatalf("recorded state after recovery = %+v", st)
	}
	if len(h.client.uploads) != 1 {
		t.Fatalf("%d uploads, want the upload not repeated once the cloud already had it", len(h.client.uploads))
	}
}

func TestDisabledSettingMakesNoCalls(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.setEnabled(false)
	h.open()

	h.pass()
	err := h.svc.SyncNow(gameLocal)

	if codeOfErr(t, err) != codeDisabled {
		t.Fatalf("SyncNow err = %v", err)
	}
	if err := h.svc.ResolveConflict(gameLocal, true); codeOfErr(t, err) != codeDisabled {
		t.Fatalf("ResolveConflict err = %v", err)
	}
	if n := h.client.calls(); n != 0 {
		t.Fatalf("%d client calls with the setting off", n)
	}
	if evs := h.drain(); len(evs) != 0 {
		t.Fatalf("events with the setting off: %+v", evs)
	}
	if st := h.svc.Status(); st.Enabled || !st.SignedIn {
		t.Fatalf("status = %+v", st)
	}
}

func TestEmptyTokenMakesNoCalls(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.setToken("")
	h.open()

	h.pass()

	if codeOfErr(t, h.svc.SyncNow(gameLocal)) != codeNotSignedIn {
		t.Fatal("SyncNow did not report that nobody is signed in")
	}
	if n := h.client.calls(); n != 0 {
		t.Fatalf("%d client calls without a token", n)
	}
	if st := h.svc.Status(); st.SignedIn {
		t.Fatalf("status = %+v", st)
	}
	if _, err := os.Stat(h.svc.store.path(ownerID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state was written without a token: %v", err)
	}
}

func TestTokenErrorIsNotTreatedAsSignedOut(t *testing.T) {
	h := newHarness(t)
	svc, err := NewService(h.dir, h.config, func() (string, error) { return "", errors.New("keychain locked") },
		func(string) (string, error) { return ownerID, nil }, h.client, h.lib, h.backups)
	if err != nil {
		t.Fatal(err)
	}
	svc.emit = func(ev Event) { h.events <- ev }
	svc.pass(context.Background(), nil)
	evs := h.drain()
	if len(evs) != 1 || evs[0].Status != StateFailed || evs[0].Code != codeToken {
		t.Fatalf("events = %+v, want a failure %s", evs, codeToken)
	}
	if h.client.calls() != 0 {
		t.Fatal("client was called after a token error")
	}
}

func TestArchiveOverTheLimitIsNotUploaded(t *testing.T) {
	h := newHarness(t)
	h.client.maxBytes = 10
	writeTree(t, h.saves(), map[string]string{"big.sav": strings.Repeat("0123456789abcdef", 64)})

	h.pass()

	if len(h.client.uploads) != 0 {
		t.Fatalf("%d uploads of an archive over the limit", len(h.client.uploads))
	}
	got := h.status(gameLocal)
	if got.State != StateFailed || got.Code != codeTooLarge {
		t.Fatalf("status = %+v, want failed %s", got, codeTooLarge)
	}
	if st := h.recorded(gameCloud); st != (gameState{}) {
		t.Fatalf("state touched: %+v", st)
	}
	entries, err := os.ReadDir(h.svc.store.tmpDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary directory has %v (err %v)", entries, err)
	}
}

func TestServerErrorCodesMapToStatusCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{"quota", &account.Error{Code: account.CodeSaveQuota, Status: 413}, codeQuota},
		{"too large", &account.Error{Code: account.CodeSaveTooLarge, Status: 413}, codeTooLarge},
		{"invalid", &account.Error{Code: account.CodeSaveInvalid, Status: 422}, codeInvalid},
		{"unauthenticated", &account.Error{Code: account.CodeUnauthenticated, Status: 401}, codeUnauthorized},
		{"rate limited", &account.Error{Code: account.CodeRateLimited, Status: 429}, codeRateLimited},
		{"network", &account.Error{Code: account.CodeNetwork}, codeNetwork},
		{"server", &account.Error{Code: account.CodeServer, Status: 500}, codeServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			writeTree(t, h.saves(), baseFiles)
			h.client.uploadErr = tc.err

			h.pass()

			got := h.status(gameLocal)
			if got.State != StateFailed || got.Code != tc.code {
				t.Fatalf("status = %+v, want failed %s", got, tc.code)
			}
			if st := h.recorded(gameCloud); st != (gameState{}) {
				t.Fatalf("a failed upload changed the record: %+v", st)
			}
		})
	}
}

func TestListFailureIsReportedForTheAccount(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.client.listErr = &account.Error{Code: account.CodeNetwork}

	h.pass()

	evs := h.drain()
	if len(evs) != 1 || evs[0].GameID != "" || evs[0].Status != StateFailed || evs[0].Code != codeNetwork {
		t.Fatalf("events = %+v", evs)
	}
	if len(h.client.uploads) != 0 {
		t.Fatal("uploaded without a listing")
	}
}

func TestSymlinkInSavesIsAnError(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	target := filepath.Join(t.TempDir(), "outside.sav")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(h.saves(), "link.sav")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot create a symlink fixture without Developer Mode or elevation: %v", err)
		}
		t.Fatal(err)
	}

	h.pass()

	if len(h.client.uploads) != 0 {
		t.Fatal("a folder with a symlink was uploaded")
	}
	got := h.status(gameLocal)
	if got.State != StateFailed || got.Code != codeSymlink {
		t.Fatalf("status = %+v, want failed %s", got, codeSymlink)
	}
}

func TestZipSlipArchiveIsRefusedAndNothingIsWritten(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"parent", "../evil.sav"},
		{"nested parent", "profile/../../evil.sav"},
		{"absolute", "/evil.sav"},
		{"backslash", `..\evil.sav`},
		{"drive", "C:/evil.sav"},
		{"empty segment", "profile//evil.sav"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			files := map[string]string{tc.entry: "evil"}
			h.client.seed(gameCloud, "other-device", zipOf(t, files), contractDigest(files), time.Now())

			h.pass()

			got := h.status(gameLocal)
			if got.State != StateFailed || got.Code != codeInvalid {
				t.Fatalf("status = %+v, want failed %s", got, codeInvalid)
			}
			if len(h.backups.snapshots) != 0 || len(h.backups.restores) != 0 {
				t.Fatalf("a hostile archive reached the backups: %+v", h.backups.snapshots)
			}
			if st := h.recorded(gameCloud); st != (gameState{}) {
				t.Fatalf("state touched: %+v", st)
			}
			var stray []string
			err := filepath.WalkDir(filepath.Dir(h.dir), func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.Contains(d.Name(), "evil") {
					stray = append(stray, p)
				}
				return err
			})
			if err != nil || len(stray) != 0 {
				t.Fatalf("files written from a hostile archive: %v (walk err %v)", stray, err)
			}
			entries, err := os.ReadDir(h.svc.store.tmpDir())
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary directory has %v (err %v)", entries, err)
			}
		})
	}
}

func TestDownloadedArchiveMustMatchItsDigest(t *testing.T) {
	h := newHarness(t)
	h.client.seed(gameCloud, "other-device", zipOf(t, baseFiles), contractDigest(otherDevice), time.Now())

	h.pass()

	got := h.status(gameLocal)
	if got.State != StateFailed || got.Code != codeDigest {
		t.Fatalf("status = %+v, want failed %s", got, codeDigest)
	}
	if len(h.backups.snapshots) != 0 {
		t.Fatal("an archive that does not match its digest reached the backups")
	}
}

func TestBackupFailureLeavesLocalSavesAndStateAlone(t *testing.T) {
	h := newHarness(t)
	h.seedRemote(baseFiles, "other-device")
	h.backups.snapshotErr = errors.New("no space")

	h.pass()

	got := h.status(gameLocal)
	if got.State != StateFailed || got.Code != codeBackup {
		t.Fatalf("status = %+v, want failed %s", got, codeBackup)
	}
	if st := h.recorded(gameCloud); st != (gameState{}) {
		t.Fatalf("state touched: %+v", st)
	}
}

func TestRotationFailureAfterTheSnapshotDoesNotBlockTheRestore(t *testing.T) {
	h := newHarness(t)
	h.seedRemote(baseFiles, "other-device")
	h.backups.afterErr = uierr.New("savebackup.rotation_failed", "old copies stay")

	h.pass()

	if got := h.status(gameLocal); got.State != StateSynced {
		t.Fatalf("status = %+v, want synced: the snapshot exists, only trimming old ones failed", got)
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves = %v", got)
	}
}

func TestRestoreRefusedWhileGameRunsIsASkip(t *testing.T) {
	h := newHarness(t)
	h.seedRemote(baseFiles, "other-device")
	h.backups.restoreErr = uierr.New("savebackup.game_running", "running")

	h.pass()

	if got := h.status(gameLocal); got.State != StateSkipped || got.Code != codeGameRunning {
		t.Fatalf("status = %+v", got)
	}
}

func TestRunningGameIsSkippedBeforeAnyTransfer(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.lib.setRunning(gameLocal, true)

	h.pass()

	if len(h.client.uploads) != 0 || h.client.downloads != 0 {
		t.Fatal("a running game was synced")
	}
	if got := h.status(gameLocal); got.State != StateSkipped || got.Code != codeGameRunning {
		t.Fatalf("status = %+v", got)
	}
	h.lib.setRunning(gameLocal, false)
	h.pass()
	if len(h.client.uploads) != 1 {
		t.Fatalf("the game was not synced once it stopped: %d uploads", len(h.client.uploads))
	}
}

func TestSavesFolderProblemsAreSkipsWithReasons(t *testing.T) {
	cases := []struct {
		name   string
		result func(h *harness)
		code   string
		state  State
	}{
		{"not found", func(h *harness) { h.lib.results[gameLocal] = library.SavesResult{} }, codeSavesMissing, StateSkipped},
		{"ambiguous", func(h *harness) { h.lib.results[gameLocal] = library.SavesResult{Candidates: []string{"a", "b"}} }, codeSavesAmbig, StateSkipped},
		{"rejected as cache", func(h *harness) {
			h.lib.results[gameLocal] = library.SavesResult{Rejected: []library.RejectedSaves{{Path: "C:/g/cache", Reason: "mostly cache"}}}
		}, codeSavesCache, StateSkipped},
		{"unreadable", func(h *harness) { h.lib.results[gameLocal] = library.SavesResult{Unreadable: 2} }, codeSavesUnread, StateFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.result(h)

			h.pass()

			got := h.status(gameLocal)
			if got.State != tc.state || got.Code != tc.code {
				t.Fatalf("status = %+v, want %s %s", got, tc.state, tc.code)
			}
			if len(h.client.uploads) != 0 {
				t.Fatal("uploaded without a saves folder")
			}
		})
	}
}

func TestGamesWithoutACloudKeyAreIgnoredByPassAndReportedBySyncNow(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.addGame("no-key", "")
	writeTree(t, h.lib.saves["no-key"], baseFiles)
	for _, id := range []string{"gone", "old"} {
		writeTree(t, h.addGame(id, "cloud-"+id), baseFiles)
	}
	h.markGame("gone", true, false)
	h.markGame("old", false, true)
	h.open()

	h.pass()

	if len(h.client.uploads) != 1 || h.client.uploads[0].key != gameCloud {
		t.Fatalf("uploads = %+v, want only the eligible game", h.client.uploads)
	}
	err := h.svc.SyncNow("no-key")
	if codeOfErr(t, err) != codeNoCanonicalID {
		t.Fatalf("SyncNow err = %v, want %s", err, codeNoCanonicalID)
	}
	if got := h.status("no-key"); got.State != StateFailed || got.Code != codeNoCanonicalID {
		t.Fatalf("status = %+v", got)
	}
	if err := h.svc.SyncNow("missing"); codeOfErr(t, err) != codeGameNotFound {
		t.Fatalf("SyncNow of an unknown game: %v", err)
	}
}

func TestResolveConflictKeepLocalOverwritesTheCloud(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	h.seedRemote(otherDevice, "other-device")
	h.pass()
	h.open()

	if err := h.svc.ResolveConflict(gameLocal, true); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}

	last := h.client.uploads[len(h.client.uploads)-1]
	if !last.force || last.ifMatch != "" {
		t.Fatalf("upload = force %v ifMatch %q, want a forced upload", last.force, last.ifMatch)
	}
	if rs, _ := h.client.remoteOf(gameCloud); rs.save.Digest != contractDigest(changedBase) {
		t.Fatalf("cloud holds %s", rs.save.Digest)
	}
	if st := h.recorded(gameCloud); st.Conflict != nil || st.LocalDigest != contractDigest(changedBase) {
		t.Fatalf("state = %+v", st)
	}
	if got := h.status(gameLocal); got.State != StateSynced {
		t.Fatalf("status = %+v", got)
	}
}

func TestResolveConflictKeepRemoteRestoresTheCloudCopy(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	remoteDigest := h.seedRemote(otherDevice, "other-device")
	h.pass()
	h.open()

	if err := h.svc.ResolveConflict(gameLocal, false); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}

	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v", got)
	}
	if st := h.recorded(gameCloud); st.Conflict != nil || st.LocalDigest != remoteDigest || st.RemoteDigest != remoteDigest {
		t.Fatalf("state = %+v", st)
	}
}

func TestResolveConflictWithoutConflictFails(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.open()
	if err := h.svc.ResolveConflict(gameLocal, true); codeOfErr(t, err) != codeNoConflict {
		t.Fatalf("err = %v", err)
	}
	if len(h.client.uploads) != 0 {
		t.Fatal("uploaded without a conflict")
	}
}

func TestDeleteRemoteWorksWithTheSettingOff(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	h.setEnabled(false)
	h.open()

	if err := h.svc.DeleteRemote(gameLocal); err != nil {
		t.Fatalf("DeleteRemote: %v", err)
	}

	if _, ok := h.client.remoteOf(gameCloud); ok {
		t.Fatal("the archive is still in the cloud")
	}
	if st := h.recorded(gameCloud); st != (gameState{Excluded: true}) {
		t.Fatalf("state = %+v, want only the exclusion", st)
	}
	reloaded, err := h.svc.store.load(ownerID)
	if err != nil || !reloaded.Games[gameCloud].Excluded {
		t.Fatalf("state file = %+v (err %v)", reloaded, err)
	}
}

func TestDeleteRemoteFailureKeepsTheRecord(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	h.client.deleteErr[gameCloud] = &account.Error{Code: account.CodeNetwork}
	h.open()

	if err := h.svc.DeleteRemote(gameLocal); err == nil {
		t.Fatal("a failed delete reported success")
	}
	if st := h.recorded(gameCloud); st.RemoteDigest == "" {
		t.Fatal("the record was dropped although the archive is still in the cloud")
	}
}

func TestForgetDeletesEverythingIncludingUnknownGames(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	h.client.seed("not-in-library", "dev", zipOf(t, baseFiles), contractDigest(baseFiles), time.Now())
	h.setEnabled(false)
	h.open()

	if err := h.svc.Forget(); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	if len(h.client.remote) != 0 {
		t.Fatalf("cloud still holds %d archives", len(h.client.remote))
	}
	reloaded, err := h.svc.store.load(ownerID)
	if err != nil || len(reloaded.Games) != 0 {
		t.Fatalf("state file = %+v (err %v)", reloaded, err)
	}
	if st := h.svc.Status(); len(st.Games) != 0 || st.UsedBytes != 0 {
		t.Fatalf("status = %+v", st)
	}
}

func TestForgetKeepsRecordsOfWhatItCouldNotDelete(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	h.client.deleteErr[gameCloud] = &account.Error{Code: account.CodeNetwork}
	h.open()

	err := h.svc.Forget()

	if err == nil {
		t.Fatal("Forget reported success although a delete failed")
	}
	if st := h.recorded(gameCloud); st.RemoteDigest == "" {
		t.Fatal("the record of an archive that is still stored was dropped")
	}
}

func TestSessionStoppedNudgesOnlyWhenEnabled(t *testing.T) {
	h := newHarness(t)
	h.svc.interval = time.Hour
	h.svc.debounce = time.Hour
	h.setEnabled(false)
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	h.svc.SessionStopped(gameLocal)
	if got := h.svc.takeDirty(); len(got) != 0 {
		t.Fatalf("dirty = %v with the setting off", got)
	}
	h.setEnabled(true)
	h.svc.SessionStopped(gameLocal)
	h.svc.SessionStopped("other")
	got := h.svc.takeDirty()
	if _, ok := got[gameLocal]; !ok || len(got) != 2 {
		t.Fatalf("dirty = %v", got)
	}
	h.svc.Nudge("")
	if got := h.svc.takeDirty(); got != nil {
		t.Fatalf("dirty = %v, want nil meaning every game", got)
	}
}

func TestLoopSyncsAtStartupAndAfterANudge(t *testing.T) {
	h := newHarness(t)
	h.svc.interval = time.Hour
	h.svc.debounce = time.Millisecond
	writeTree(t, h.saves(), baseFiles)
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	first := waitEvent(t, h.events)
	if first.Status != StateSynced || first.GameID != gameLocal {
		t.Fatalf("first event = %+v", first)
	}

	writeTree(t, h.saves(), changedBase)
	h.svc.SessionStopped(gameLocal)
	second := waitEvent(t, h.events)
	if second.Status != StateSynced {
		t.Fatalf("second event = %+v", second)
	}
	h.client.mu.Lock()
	n := len(h.client.uploads)
	h.client.mu.Unlock()
	if n != 2 {
		t.Fatalf("%d uploads, want 2", n)
	}
}

func TestStartupSweepsInterruptedTemporaryFiles(t *testing.T) {
	h := newHarness(t)
	h.svc.interval = time.Hour
	leftover := filepath.Join(h.svc.store.tmpDir(), "game-123", "save.zip")
	writeTree(t, filepath.Dir(leftover), map[string]string{"save.zip": "partial"})
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leftover survived startup: %v", err)
	}
}

func TestConcurrentNudgeSyncNowStatusAndShutdown(t *testing.T) {
	h := newHarness(t)
	h.svc.interval = time.Millisecond
	h.svc.debounce = time.Millisecond
	writeTree(t, h.saves(), baseFiles)
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for range 20 {
				h.svc.Nudge(gameLocal)
				h.svc.SessionStopped(gameLocal)
			}
		}()
		go func() {
			defer wg.Done()
			for range 5 {
				if err := h.svc.SyncNow(gameLocal); err != nil && !errors.Is(err, errNotStarted) && classify(err) == codeFailed {
					t.Errorf("SyncNow: %v", err)
				}
				h.svc.Status()
			}
		}()
		go func() {
			defer wg.Done()
			if i == 3 {
				if err := h.svc.ServiceShutdown(); err != nil {
					t.Errorf("shutdown: %v", err)
				}
			}
			h.svc.Status()
		}()
	}
	wg.Wait()
	if err := h.svc.ServiceShutdown(); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	if err := h.svc.SyncNow(gameLocal); !errors.Is(err, errNotStarted) {
		t.Fatalf("SyncNow after shutdown = %v, want errNotStarted", err)
	}
}

func TestNewServiceRejectsMissingPieces(t *testing.T) {
	h := newHarness(t)
	ok := func(string) (string, error) { return ownerID, nil }
	tok := func() (string, error) { return "t", nil }
	cases := []struct {
		name string
		dir  string
		call func(dir string) (*Service, error)
	}{
		{"empty dir", "", func(d string) (*Service, error) {
			return NewService(d, h.config, tok, ok, h.client, h.lib, h.backups)
		}},
		{"relative dir", "relative", func(d string) (*Service, error) {
			return NewService(d, h.config, tok, ok, h.client, h.lib, h.backups)
		}},
		{"no settings", h.dir, func(d string) (*Service, error) {
			return NewService(d, nil, tok, ok, h.client, h.lib, h.backups)
		}},
		{"no token", h.dir, func(d string) (*Service, error) {
			return NewService(d, h.config, nil, ok, h.client, h.lib, h.backups)
		}},
		{"no account", h.dir, func(d string) (*Service, error) {
			return NewService(d, h.config, tok, nil, h.client, h.lib, h.backups)
		}},
		{"no client", h.dir, func(d string) (*Service, error) {
			return NewService(d, h.config, tok, ok, nil, h.lib, h.backups)
		}},
		{"no library", h.dir, func(d string) (*Service, error) {
			return NewService(d, h.config, tok, ok, h.client, nil, h.backups)
		}},
		{"no backups", h.dir, func(d string) (*Service, error) {
			return NewService(d, h.config, tok, ok, h.client, h.lib, nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if svc, err := tc.call(tc.dir); err == nil || svc != nil {
				t.Fatalf("NewService accepted %q", tc.name)
			}
		})
	}
}

func TestStatusCarriesUsageFromTheLastListing(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	st := h.svc.Status()
	if !st.Enabled || !st.SignedIn || st.QuotaBytes != 1<<30 || st.UsedBytes == 0 || st.MaxBytes != h.client.maxBytes {
		t.Fatalf("status = %+v", st)
	}
	if !slices.IsSortedFunc(st.Games, func(a, b GameStatus) int { return strings.Compare(a.GameID, b.GameID) }) {
		t.Fatalf("games not sorted: %+v", st.Games)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func waitEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("no event within 10s")
		return Event{}
	}
}

func TestUploadAfterRemoteDeletionRetriesWithoutIfMatch(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	h.client.mu.Lock()
	delete(h.client.remote, gameCloud)
	h.client.mu.Unlock()
	h.client.beforeSend = nil
	// The listing still shows the record, as if it was deleted right after it.
	h.client.seed(gameCloud, "dev", zipOf(t, baseFiles), contractDigest(baseFiles), time.Now())
	h.client.beforeSend = func() {
		h.client.mu.Lock()
		delete(h.client.remote, gameCloud)
		h.client.mu.Unlock()
		h.client.beforeSend = nil
	}

	h.pass()

	if len(h.client.uploads) != 3 {
		t.Fatalf("%d uploads, want the first, the refused one and the retry", len(h.client.uploads))
	}
	first, retry := h.client.uploads[1], h.client.uploads[2]
	if first.ifMatch == "" || retry.ifMatch != "" || retry.force {
		t.Fatalf("second attempt ifMatch %q, retry ifMatch %q force %v", first.ifMatch, retry.ifMatch, retry.force)
	}
	want := contractDigest(changedBase)
	if st := h.recorded(gameCloud); st.LocalDigest != want || st.RemoteDigest != want || st.Conflict != nil {
		t.Fatalf("recorded state = %+v", st)
	}
	if got := h.status(gameLocal); got.State != StateSynced {
		t.Fatalf("status = %+v", got)
	}
}

func TestRepeatedConflictWithoutRecordIsAFailure(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	writeTree(t, h.saves(), changedBase)
	h.client.alwaysClash = true

	h.pass()

	if len(h.client.uploads) != 3 {
		t.Fatalf("%d uploads, want exactly one retry", len(h.client.uploads))
	}
	got := h.status(gameLocal)
	if got.State != StateFailed || got.Code != codeConflictRetry {
		t.Fatalf("status = %+v, want failed %s", got, codeConflictRetry)
	}
	if st := h.recorded(gameCloud); st.Conflict != nil || st.LocalDigest != contractDigest(baseFiles) {
		t.Fatalf("recorded state = %+v, want no conflict and the old local digest", st)
	}
}

func TestBusyServerIsPendingAndRetriedOnTheNextPass(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(h *harness)
		heal func(h *harness)
	}{
		{"upload", func(h *harness) {
			writeTree(t, h.saves(), baseFiles)
			h.client.uploadErr = &account.Error{Code: account.CodeSaveBusy, Status: 503}
		}, func(h *harness) { h.client.uploadErr = nil }},
		{"download", func(h *harness) {
			h.seedRemote(baseFiles, "other")
			h.client.downloadErr = &account.Error{Code: account.CodeSaveBusy, Status: 503}
		}, func(h *harness) { h.client.downloadErr = nil }},
		{"list", func(h *harness) {
			writeTree(t, h.saves(), baseFiles)
			h.client.listErr = &account.Error{Code: account.CodeSaveBusy, Status: 503}
		}, func(h *harness) { h.client.listErr = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.arm(h)

			h.pass()

			evs := h.drain()
			if len(evs) != 1 || evs[0].Status != StatePending || evs[0].Code != codeBusy {
				t.Fatalf("events = %+v, want one pending %s", evs, codeBusy)
			}
			if st := h.recorded(gameCloud); st != (gameState{}) {
				t.Fatalf("state touched: %+v", st)
			}
			tc.heal(h)
			h.pass()
			if got := h.status(gameLocal); got.State != StateSynced {
				t.Fatalf("status after the retry = %+v", got)
			}
		})
	}
}

func TestDeleteRemoteExcludesTheGameUntilItIsSyncedByHand(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	h.open()
	if err := h.svc.DeleteRemote(gameLocal); err != nil {
		t.Fatalf("DeleteRemote: %v", err)
	}
	uploads := len(h.client.uploads)

	h.pass()
	h.pass()

	if len(h.client.uploads) != uploads || h.client.downloads != 0 {
		t.Fatalf("a pass moved data for an excluded game: uploads %d downloads %d", len(h.client.uploads)-uploads, h.client.downloads)
	}
	if _, ok := h.client.remoteOf(gameCloud); ok {
		t.Fatal("the deleted archive came back")
	}
	if got := h.status(gameLocal); got.State != StateSkipped || got.Code != codeExcluded {
		t.Fatalf("status = %+v, want skipped %s", got, codeExcluded)
	}

	if err := h.svc.SyncNow(gameLocal); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if len(h.client.uploads) != uploads+1 {
		t.Fatalf("SyncNow did not upload: %d uploads", len(h.client.uploads)-uploads)
	}
	if st := h.recorded(gameCloud); st.Excluded || st.RemoteDigest != contractDigest(baseFiles) {
		t.Fatalf("state = %+v, want the exclusion lifted and the upload recorded", st)
	}
	writeTree(t, h.saves(), changedBase)
	h.pass()
	if len(h.client.uploads) != uploads+2 {
		t.Fatal("passes did not resume after the game was synced by hand")
	}
}

func TestDuplicateCatalogEntriesAreNeverSynced(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	writeTree(t, h.addGame("local-2", gameCloud), changedBase)
	h.open()

	h.pass()

	if h.client.calls() != 1 {
		t.Fatalf("%d client calls, want only the listing", h.client.calls())
	}
	for _, id := range []string{gameLocal, "local-2"} {
		if got := h.status(id); got.State != StateSkipped || got.Code != codeDuplicate {
			t.Fatalf("status of %s = %+v, want skipped %s", id, got, codeDuplicate)
		}
	}
	for _, call := range []func() error{
		func() error { return h.svc.SyncNow(gameLocal) },
		func() error { return h.svc.PullRemote("local-2") },
		func() error { return h.svc.ResolveConflict(gameLocal, true) },
	} {
		if codeOfErr(t, call()) != codeDuplicate {
			t.Fatal("a manual action ran for a duplicated entry")
		}
	}
	if len(h.client.uploads) != 0 || h.client.downloads != 0 {
		t.Fatal("data moved for a duplicated entry")
	}

	h.markGame("local-2", false, true)
	h.pass()
	if len(h.client.uploads) != 1 || h.client.uploads[0].digest != contractDigest(baseFiles) {
		t.Fatalf("uploads = %+v, want the one live entry synced once the other is archived", h.client.uploads)
	}
}

func TestEmptyLocalFolderIsRestoredFromTheCloudWhateverTheRecord(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	if err := os.RemoveAll(h.saves()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(h.saves(), 0o700); err != nil {
		t.Fatal(err)
	}

	h.pass()

	if len(h.client.uploads) != 1 {
		t.Fatalf("%d uploads: an empty folder must not replace the cloud copy", len(h.client.uploads))
	}
	if h.client.downloads != 1 || len(h.backups.restores) != 1 {
		t.Fatalf("downloads %d restores %d, want the cloud copy restored", h.client.downloads, len(h.backups.restores))
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves = %v", got)
	}
}

func TestMissingSavesDirUsesThePlayersFolderNotDetection(t *testing.T) {
	h := newHarness(t)
	chosen := filepath.Join(t.TempDir(), "ChosenSaves")
	detected := t.TempDir()
	writeTree(t, detected, map[string]string{"other.sav": "must never be uploaded"})
	h.lib.mu.Lock()
	h.lib.games[0].SavesDir = chosen
	h.lib.saves[gameLocal] = chosen
	h.lib.results[gameLocal] = library.SavesResult{Path: detected}
	h.lib.mu.Unlock()
	h.seedRemote(baseFiles, "other-device")

	h.pass()

	if len(h.client.uploads) != 0 {
		t.Fatalf("the detected folder was uploaded: %+v", h.client.uploads)
	}
	if got := readTree(t, chosen); !mapsEqual(got, baseFiles) {
		t.Fatalf("chosen folder = %v, want the cloud copy", got)
	}
	if got := h.status(gameLocal); got.State != StateSynced {
		t.Fatalf("status = %+v", got)
	}
}

func TestMissingSavesDirWithNothingInTheCloudCreatesNothing(t *testing.T) {
	h := newHarness(t)
	chosen := filepath.Join(t.TempDir(), "ChosenSaves")
	h.lib.mu.Lock()
	h.lib.games[0].SavesDir = chosen
	h.lib.mu.Unlock()

	h.pass()

	if _, err := os.Stat(chosen); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the folder was created without anything to restore: %v", err)
	}
	if got := h.status(gameLocal); got.State != StateSkipped || got.Code != codeEmpty {
		t.Fatalf("status = %+v", got)
	}
}

func TestSavesDirThatIsAFileIsAnError(t *testing.T) {
	h := newHarness(t)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.lib.mu.Lock()
	h.lib.games[0].SavesDir = file
	h.lib.mu.Unlock()

	h.pass()

	if got := h.status(gameLocal); got.State != StateFailed || got.Code != codeSavesNotDir {
		t.Fatalf("status = %+v, want failed %s", got, codeSavesNotDir)
	}
}

func TestPullRemoteTakesTheCloudCopyWithoutAConflict(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), changedBase)
	h.seedRemote(otherDevice, "other-device")
	h.open()

	if err := h.svc.PullRemote(gameLocal); err != nil {
		t.Fatalf("PullRemote: %v", err)
	}

	if got := readTree(t, h.saves()); !mapsEqual(got, otherDevice) {
		t.Fatalf("saves = %v", got)
	}
	if len(h.backups.snapshots) != 1 || h.backups.snapshots[0].kind != savebackup.KindCloud || len(h.backups.restores) != 1 {
		t.Fatalf("snapshots %+v restores %v, want the cloud copy restored through savebackup", h.backups.snapshots, h.backups.restores)
	}
	want := contractDigest(otherDevice)
	if st := h.recorded(gameCloud); st.LocalDigest != want || st.RemoteDigest != want || st.Conflict != nil {
		t.Fatalf("state = %+v", st)
	}
	if len(h.client.uploads) != 0 {
		t.Fatal("PullRemote uploaded")
	}
}

func TestPullRemoteWithNothingInTheCloud(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.open()

	err := h.svc.PullRemote(gameLocal)

	if codeOfErr(t, err) != codeNoRemote {
		t.Fatalf("err = %v, want %s", err, codeNoRemote)
	}
	if h.client.downloads != 0 || len(h.backups.restores) != 0 {
		t.Fatal("PullRemote ran without a cloud copy")
	}
	if got := readTree(t, h.saves()); !mapsEqual(got, baseFiles) {
		t.Fatalf("saves changed to %v", got)
	}
}

func TestEnablingTheSettingNudgesOnlyOnTheFalseToTrueEdge(t *testing.T) {
	h := newHarness(t)
	h.setEnabled(false)
	h.svc.enabled = false
	h.svc.interval = time.Hour
	h.svc.debounce = time.Hour
	h.svc.retryDelay = time.Hour
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	dirty := func() int {
		h.svc.mu.Lock()
		defer h.svc.mu.Unlock()
		return len(h.svc.dirty)
	}

	h.svc.SettingsChanged(h.config())
	if dirty() != 0 {
		t.Fatal("a change that left the setting off queued a pass")
	}
	h.setEnabled(true)
	h.svc.SettingsChanged(h.config())
	if dirty() != 1 {
		t.Fatalf("turning the setting on queued %d requests, want 1", dirty())
	}
	h.svc.takeDirty()
	h.svc.SettingsChanged(h.config())
	if dirty() != 0 {
		t.Fatal("an unrelated settings change queued a pass")
	}
	h.setEnabled(false)
	h.svc.SettingsChanged(h.config())
	if dirty() != 0 {
		t.Fatal("turning the setting off queued a pass")
	}
}

func TestEnablingTheSettingSyncsWithoutWaitingForTheTimer(t *testing.T) {
	h := newHarness(t)
	h.setEnabled(false)
	h.svc.enabled = false
	h.svc.interval = time.Hour
	h.svc.debounce = time.Millisecond
	writeTree(t, h.saves(), baseFiles)
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	h.setEnabled(true)
	h.svc.SettingsChanged(h.config())

	if ev := waitEvent(t, h.events); ev.Status != StateSynced || ev.GameID != gameLocal {
		t.Fatalf("event = %+v", ev)
	}
}

func TestUnknownAccountIsPendingAndRetriedWithoutAGlobalError(t *testing.T) {
	h := newHarness(t)
	h.setAccountErr(errors.New("not bound yet"))
	h.svc.interval = time.Hour
	h.svc.debounce = time.Hour
	h.svc.retryDelay = time.Millisecond
	writeTree(t, h.saves(), baseFiles)
	if err := h.svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.svc.ServiceShutdown(); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})

	first := waitEvent(t, h.events)
	if first.GameID != "" || first.Status != StatePending || first.Code != codeAccountUnk {
		t.Fatalf("first event = %+v, want pending %s", first, codeAccountUnk)
	}
	if st := h.svc.Status(); st.Error != "" || !st.Pending {
		t.Fatalf("status = %+v: an unbound account is pending, not a failure", st)
	}
	if h.client.calls() != 0 {
		t.Fatal("the cloud was contacted before the account was known")
	}

	h.setAccountErr(nil)
	next := waitEvent(t, h.events)
	if next.Status != StateSynced || next.GameID != gameLocal {
		t.Fatalf("next event = %+v, want the retry to sync the game (a repeated pending event means no dedupe)", next)
	}
	if st := h.svc.Status(); st.Pending {
		t.Fatalf("status = %+v: the account is bound, nothing is pending", st)
	}
}

func TestTransfersCarryTheirOwnDeadline(t *testing.T) {
	h := newHarness(t)
	writeTree(t, h.saves(), baseFiles)
	h.pass()
	h.seedRemote(otherDevice, "other-device")
	h.pass()

	if len(h.client.deadlines) != 2 {
		t.Fatalf("deadlines = %v, want one upload and one download", h.client.deadlines)
	}
	for _, d := range h.client.deadlines {
		if d <= 10*time.Minute || d > transferTimeout {
			t.Fatalf("transfer deadline in %v, want just under %v", d, transferTimeout)
		}
	}
}

func countBuilds(h *harness) *int {
	n := new(int)
	h.svc.build = func(ctx context.Context, dir string, files []treeFile, maxBytes int64) (archive, error) {
		*n++
		return buildArchive(ctx, dir, files, maxBytes)
	}
	return n
}

func TestRefusedArchiveIsNotRebuiltUntilFilesChange(t *testing.T) {
	big := strings.Repeat("0123456789abcdef", 64)
	later := time.Now().Add(time.Hour)
	tests := []struct {
		name       string
		change     func(t *testing.T, saves string)
		wantBuilds int
	}{
		{"nothing changed", func(*testing.T, string) {}, 1},
		{"mtime changed", func(t *testing.T, saves string) {
			if err := os.Chtimes(filepath.Join(saves, "big.sav"), later, later); err != nil {
				t.Fatal(err)
			}
		}, 2},
		{"size changed", func(t *testing.T, saves string) {
			writeTree(t, saves, map[string]string{"big.sav": big + "x"})
		}, 2},
		{"file added", func(t *testing.T, saves string) {
			writeTree(t, saves, map[string]string{"extra.sav": big})
		}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.client.maxBytes = 10
			builds := countBuilds(h)
			writeTree(t, h.saves(), map[string]string{"big.sav": big})

			h.pass()
			tt.change(t, h.saves())
			h.pass()

			if *builds != tt.wantBuilds {
				t.Fatalf("archives built = %d, want %d", *builds, tt.wantBuilds)
			}
			got := h.status(gameLocal)
			if got.State != StateFailed || got.Code != codeTooLarge || got.Message == "" {
				t.Fatalf("status = %+v, want failed %s with a message", got, codeTooLarge)
			}
		})
	}
}

func TestRefusedArchiveIsRetriedWhenTheLimitGrows(t *testing.T) {
	h := newHarness(t)
	h.client.maxBytes = 10
	builds := countBuilds(h)
	writeTree(t, h.saves(), map[string]string{"big.sav": strings.Repeat("0123456789abcdef", 64)})

	h.pass()
	h.client.maxBytes = 1 << 20
	h.pass()

	if *builds != 2 || len(h.client.uploads) != 1 {
		t.Fatalf("builds = %d, uploads = %d, want 2 and 1", *builds, len(h.client.uploads))
	}
	if got := h.status(gameLocal); got.State != StateSynced {
		t.Fatalf("status = %+v, want synced", got)
	}
}

func TestSavesOverTheUnpackedCapAreRefusedWithoutArchiving(t *testing.T) {
	h := newHarness(t)
	h.client.maxBytes = 1 << 20
	builds := countBuilds(h)
	r, err := h.svc.prepare(context.Background(), true)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	out := h.svc.upload(context.Background(), r, gameCloud, tree{size: maxUnpackedBytes + 1, sig: "s"}, action{kind: actUpload})

	if *builds != 0 || out.state != StateFailed || out.codeOf() != codeTooLarge {
		t.Fatalf("builds = %d, outcome = %+v, want no build and failed %s", *builds, out, codeTooLarge)
	}
}

func TestUnreadableSavesStayVisibleWithoutRepeatedLogs(t *testing.T) {
	h := newHarness(t)
	h.lib.results[gameLocal] = library.SavesResult{Unreadable: 1}
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h.pass()
	h.pass()
	h.pass()

	if n := strings.Count(buf.String(), "code="+codeSavesUnread); n != 1 {
		t.Fatalf("logged %d times, want once:\n%s", n, buf.String())
	}
	got := h.status(gameLocal)
	if got.State != StateFailed || got.Code != codeSavesUnread || got.Message == "" {
		t.Fatalf("status = %+v, want failed %s with a message", got, codeSavesUnread)
	}
}
