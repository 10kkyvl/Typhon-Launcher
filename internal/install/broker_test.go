package install

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func brokerDirs(t *testing.T) (dir string, pin brokerPin) {
	t.Helper()
	root := t.TempDir()
	dir = filepath.Join(root, "broker")
	pin = brokerPin{
		DownloadID:  "d1",
		ContentRoot: filepath.Join(root, "downloads", "game"),
		LibraryRoot: filepath.Join(root, "games"),
		StateDir:    filepath.Join(root, "state"),
	}
	for _, d := range []string{dir, pin.ContentRoot, pin.LibraryRoot, pin.StateDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := writeBrokerPin(brokerPinPath(dir), pin); err != nil {
		t.Fatalf("write pin: %v", err)
	}
	if err := touchBrokerAlive(brokerAlivePath(dir)); err != nil {
		t.Fatalf("touch alive: %v", err)
	}
	return dir, pin
}

func goodSpec(pin brokerPin) workerSpec {
	return workerSpec{
		ID:            "i1",
		InstallerPath: filepath.Join(pin.ContentRoot, "setup.exe"),
		WorkingDir:    pin.ContentRoot,
		Destination:   filepath.Join(pin.LibraryRoot, "Game"),
		StatePath:     filepath.Join(pin.StateDir, "state.json"),
		CancelPath:    filepath.Join(pin.StateDir, "cancel"),
		LogPath:       filepath.Join(pin.StateDir, "install.log"),
	}
}

func TestValidateBrokerSpecAcceptsPinnedPaths(t *testing.T) {
	_, pin := brokerDirs(t)
	if err := validateBrokerSpec(pin, goodSpec(pin)); err != nil {
		t.Fatalf("годная спека отвергнута: %v", err)
	}
}

func TestValidateBrokerSpecRejectsEscapes(t *testing.T) {
	_, pin := brokerDirs(t)
	outside := filepath.Join(filepath.Dir(pin.ContentRoot), "..", "evil")
	cases := []struct {
		name  string
		mutir func(*workerSpec)
	}{
		{"установщик вне каталога загрузки", func(s *workerSpec) { s.InstallerPath = filepath.Join(outside, "payload.exe") }},
		{"рабочий каталог вне каталога загрузки", func(s *workerSpec) { s.WorkingDir = outside }},
		{"установка вне папки игр", func(s *workerSpec) { s.Destination = filepath.Join(outside, "Game") }},
		{"состояние вне каталога состояний", func(s *workerSpec) { s.StatePath = filepath.Join(outside, "state.json") }},
		{"отмена вне каталога состояний", func(s *workerSpec) { s.CancelPath = filepath.Join(outside, "cancel") }},
		{"лог вне каталога состояний", func(s *workerSpec) { s.LogPath = filepath.Join(outside, "install.log") }},
		{"без установщика", func(s *workerSpec) { s.InstallerPath = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := goodSpec(pin)
			tc.mutir(&spec)
			err := validateBrokerSpec(pin, spec)
			if !errors.Is(err, errBrokerOutsidePin) {
				t.Fatalf("ожидался отказ по границам, получено %v", err)
			}
		})
	}
}

func TestReadBrokerPinRejectsMissingAndIncomplete(t *testing.T) {
	dir := t.TempDir()
	if _, err := readBrokerPin(brokerPinPath(dir)); !errors.Is(err, errBrokerNoPin) {
		t.Fatalf("отсутствующий пин: ожидался errBrokerNoPin, получено %v", err)
	}
	if err := writeBrokerPin(brokerPinPath(dir), brokerPin{DownloadID: "d1"}); err != nil {
		t.Fatalf("write pin: %v", err)
	}
	if _, err := readBrokerPin(brokerPinPath(dir)); !errors.Is(err, errBrokerNoPin) {
		t.Fatalf("пин без границ: ожидался errBrokerNoPin, получено %v", err)
	}
}

func shortBrokerTiming(t *testing.T, grace, lifetime time.Duration) {
	t.Helper()
	poll, oldGrace, oldLife := brokerPollInterval, brokerHeartbeatGrace, brokerMaxLifetime
	brokerPollInterval = time.Millisecond
	brokerHeartbeatGrace = grace
	brokerMaxLifetime = lifetime
	t.Cleanup(func() {
		brokerPollInterval, brokerHeartbeatGrace, brokerMaxLifetime = poll, oldGrace, oldLife
	})
}

func TestRunBrokerExitsOnAbort(t *testing.T) {
	dir, _ := brokerDirs(t)
	shortBrokerTiming(t, time.Hour, time.Hour)
	if err := writeWorkerFile(brokerAbortPath(dir), []byte{}); err != nil {
		t.Fatalf("write abort: %v", err)
	}
	outcome, err := RunBroker(dir)
	if err != nil {
		t.Fatalf("RunBroker: %v", err)
	}
	if outcome != BrokerAborted {
		t.Fatalf("исход = %q, ожидался %q", outcome, BrokerAborted)
	}
}

func TestRunBrokerExitsWhenLauncherStopsBeating(t *testing.T) {
	dir, _ := brokerDirs(t)
	shortBrokerTiming(t, time.Millisecond, time.Hour)
	stale := time.Now().Add(-time.Hour)
	if err := os.Chtimes(brokerAlivePath(dir), stale, stale); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	outcome, err := RunBroker(dir)
	if err != nil {
		t.Fatalf("RunBroker: %v", err)
	}
	if outcome != BrokerAbandoned {
		t.Fatalf("исход = %q, ожидался %q", outcome, BrokerAbandoned)
	}
}

func TestRunBrokerExitsWhenHeartbeatFileIsGone(t *testing.T) {
	dir, _ := brokerDirs(t)
	shortBrokerTiming(t, time.Hour, time.Hour)
	if err := os.Remove(brokerAlivePath(dir)); err != nil {
		t.Fatalf("remove alive: %v", err)
	}
	outcome, err := RunBroker(dir)
	if err != nil {
		t.Fatalf("RunBroker: %v", err)
	}
	if outcome != BrokerAbandoned {
		t.Fatalf("исход = %q, ожидался %q", outcome, BrokerAbandoned)
	}
}

func TestRunBrokerExpires(t *testing.T) {
	dir, _ := brokerDirs(t)
	shortBrokerTiming(t, time.Hour, 0)
	outcome, err := RunBroker(dir)
	if err != nil {
		t.Fatalf("RunBroker: %v", err)
	}
	if outcome != BrokerExpired {
		t.Fatalf("исход = %q, ожидался %q", outcome, BrokerExpired)
	}
}

// Задание вне границ не просто отклоняется — брокер обязан уйти, а не остаться
// висеть с правами администратора в ожидании следующей попытки.
func TestRunBrokerRefusesSpecOutsidePin(t *testing.T) {
	dir, pin := brokerDirs(t)
	shortBrokerTiming(t, time.Hour, time.Hour)
	spec := goodSpec(pin)
	spec.InstallerPath = filepath.Join(t.TempDir(), "payload.exe")
	if err := writeWorkerSpec(brokerSpecPath(dir), spec); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	outcome, err := RunBroker(dir)
	if !errors.Is(err, errBrokerOutsidePin) {
		t.Fatalf("ожидался отказ по границам, получено %v", err)
	}
	if outcome != BrokerAborted {
		t.Fatalf("исход = %q, ожидался %q", outcome, BrokerAborted)
	}
}

var brokerTestPrivate = ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
var brokerTestPublic = base64.StdEncoding.EncodeToString(brokerTestPrivate.Public().(ed25519.PublicKey))
