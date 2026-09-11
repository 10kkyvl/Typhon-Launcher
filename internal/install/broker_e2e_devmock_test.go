//go:build devmock && !windows

package install

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestBrokerEndToEndInProcess доказывает главное обещание схемы: права
// запрошены заранее, установка приходит часами позже и проходит через уже
// поднятый процесс, а не через новый запрос UAC. RunBroker здесь настоящий —
// он сам читает пин, дожидается задания и выполняет его тем же кодом, что и
// обычный воркер.
func TestBrokerEndToEndInProcess(t *testing.T) {
	t.Setenv(devmockInstallSecondsEnv, "0")

	root := t.TempDir()
	brokerDir := filepath.Join(root, "broker")
	contentRoot := filepath.Join(root, "download")
	stateDir := filepath.Join(root, "state")
	gamesRoot := filepath.Join(root, "Games")
	for _, d := range []string{brokerDir, contentRoot, stateDir, gamesRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	pin := brokerPin{
		DownloadID:  "d1",
		ContentRoot: contentRoot,
		LibraryRoot: gamesRoot,
		StateDir:    stateDir,
	}
	if err := writeBrokerPin(brokerPinPath(brokerDir), pin); err != nil {
		t.Fatalf("write pin: %v", err)
	}
	if err := touchBrokerAlive(brokerAlivePath(brokerDir)); err != nil {
		t.Fatalf("touch alive: %v", err)
	}

	shortBrokerTiming(t, time.Hour, time.Hour)

	var wg sync.WaitGroup
	outcomes := make(chan BrokerOutcome, 1)
	errs := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		outcome, err := RunBroker(brokerDir, brokerTestPublic)
		outcomes <- outcome
		errs <- err
	}()
	t.Cleanup(func() {
		if err := writeWorkerFile(brokerAbortPath(brokerDir), []byte{}); err != nil {
			t.Errorf("write abort: %v", err)
		}
		wg.Wait()
	})

	installerPath := filepath.Join(contentRoot, "FooGame-setup.exe")
	if err := os.WriteFile(installerPath, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(gamesRoot, "FooGame")
	gone := make(chan struct{})
	spec := runSpec{
		Path: installerPath, InstallerPath: installerPath, ID: "e2eb",
		Engine: EngineInno, Destination: dest, Dir: contentRoot,
		StatePath:  filepath.Join(stateDir, "state.json"),
		CancelPath: filepath.Join(stateDir, "cancel"),
		InfPath:    filepath.Join(stateDir, "discover.ini"),
		Broker:     &brokerHandoff{Key: brokerTestPrivate, Dir: brokerDir, Gone: gone},
	}

	spawned := false
	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		spawned = true
		return nil, errWorkerStatePath
	})

	code, err := runElevated(context.Background(), spec)
	if err != nil {
		t.Fatalf("runElevated: %v", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, ожидался 0", code)
	}
	if spawned {
		t.Fatal("поднят новый повышенный воркер, хотя брокер уже был")
	}

	if _, statErr := os.Stat(filepath.Join(dest, "FooGame.exe")); statErr != nil {
		t.Fatalf("установленный exe: %v", statErr)
	}
	if _, statErr := os.Stat(brokerSpecPath(brokerDir)); !os.IsNotExist(statErr) {
		t.Fatalf("задание не снято брокером: %v", statErr)
	}

	state, found, err := readWorkerState(spec.StatePath)
	if err != nil {
		t.Fatalf("readWorkerState: %v", err)
	}
	if !found || !state.Done || state.Error != "" {
		t.Fatalf("итоговое состояние = %+v, ожидалось done без ошибки", state)
	}
}

// Цепочка установщиков обслуживается тем же брокером: выйти после первого
// значило бы потребовать UAC на втором, когда пользователя уже нет.
func TestBrokerServesTheWholeChain(t *testing.T) {
	t.Setenv(devmockInstallSecondsEnv, "0")

	root := t.TempDir()
	brokerDir := filepath.Join(root, "broker")
	contentRoot := filepath.Join(root, "download")
	stateDir := filepath.Join(root, "state")
	gamesRoot := filepath.Join(root, "Games")
	for _, d := range []string{brokerDir, contentRoot, stateDir, gamesRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	pin := brokerPin{DownloadID: "d1", ContentRoot: contentRoot, LibraryRoot: gamesRoot, StateDir: stateDir}
	if err := writeBrokerPin(brokerPinPath(brokerDir), pin); err != nil {
		t.Fatalf("write pin: %v", err)
	}
	if err := touchBrokerAlive(brokerAlivePath(brokerDir)); err != nil {
		t.Fatalf("touch alive: %v", err)
	}

	shortBrokerTiming(t, time.Hour, time.Hour)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := RunBroker(brokerDir, brokerTestPublic); err != nil {
			t.Errorf("RunBroker: %v", err)
		}
	}()
	t.Cleanup(func() {
		if err := writeWorkerFile(brokerAbortPath(brokerDir), []byte{}); err != nil {
			t.Errorf("write abort: %v", err)
		}
		wg.Wait()
	})

	withWorkerSeams(t, func(runSpec) (workerHandle, error) {
		t.Error("поднят новый повышенный воркер вместо живого брокера")
		return nil, errWorkerStatePath
	})

	gone := make(chan struct{})
	for _, name := range []string{"FooGame", "BarGame"} {
		installerPath := filepath.Join(contentRoot, name+"-setup.exe")
		if err := os.WriteFile(installerPath, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(gamesRoot, name)
		spec := runSpec{
			Path: installerPath, InstallerPath: installerPath, ID: "chain-" + name,
			Engine: EngineInno, Destination: dest, Dir: contentRoot,
			StatePath:  filepath.Join(stateDir, "state.json"),
			CancelPath: filepath.Join(stateDir, "cancel"),
			InfPath:    filepath.Join(stateDir, "discover.ini"),
			Broker:     &brokerHandoff{Key: brokerTestPrivate, Dir: brokerDir, Gone: gone},
		}
		code, err := runElevated(context.Background(), spec)
		if err != nil {
			t.Fatalf("runElevated %s: %v", name, err)
		}
		if code != 0 {
			t.Fatalf("%s: code = %d, ожидался 0", name, code)
		}
		if _, statErr := os.Stat(filepath.Join(dest, name+".exe")); statErr != nil {
			t.Fatalf("%s не установлен: %v", name, statErr)
		}
	}
}
