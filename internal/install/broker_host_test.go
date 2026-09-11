package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/download"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeProc struct {
	exit chan struct{}
	done chan struct{}
}

func newFakeProc() *fakeProc {
	return &fakeProc{exit: make(chan struct{}), done: make(chan struct{})}
}

func (p *fakeProc) wait() (int, error) {
	<-p.exit
	return 0, nil
}

func (p *fakeProc) close() {
	select {
	case <-p.done:
	default:
		close(p.done)
		close(p.exit)
	}
}

func (p *fakeProc) terminate() error {
	p.close()
	return nil
}

func fakeElevation(t *testing.T) (started chan runSpec, procs chan *fakeProc) {
	t.Helper()
	started = make(chan runSpec, 4)
	procs = make(chan *fakeProc, 4)
	old, oldAvail, oldStop := startElevatedWorker, elevationAvailable, brokerStopWait
	live := make([]*fakeProc, 0, 4)
	startElevatedWorker = func(spec runSpec) (workerHandle, error) {
		p := newFakeProc()
		live = append(live, p)
		started <- spec
		procs <- p
		return p, nil
	}
	elevationAvailable = func() bool { return true }
	// Настоящий брокер уходит по метке abort; фейковый её не читает, поэтому
	// его выход имитируется здесь — иначе выключение сервиса ждало бы
	// brokerStopWait целиком, и горутина ожидания осталась бы висеть.
	brokerStopWait = 50 * time.Millisecond
	t.Cleanup(func() {
		for _, p := range live {
			p.close()
		}
		startElevatedWorker, elevationAvailable, brokerStopWait = old, oldAvail, oldStop
	})
	return started, procs
}

func startedDownload(t *testing.T, s *Service) download.Download {
	t.Helper()
	dest := t.TempDir()
	yes := true
	return download.Download{
		ID:          "d1",
		Destination: dest,
		Files:       []download.FileState{{Path: "setup.exe", Selected: true}},
		Origin:      download.Origin{AutoInstall: &yes, ElevateAhead: true},
	}
}

func TestHandleDownloadStartedRaisesBroker(t *testing.T) {
	s, _, _ := newTestService(t)
	started, _ := fakeElevation(t)

	s.HandleDownloadStarted(startedDownload(t, s))

	select {
	case spec := <-started:
		if len(spec.Args) != 3 || spec.Args[0] != installBrokerFlag {
			t.Fatalf("брокер запущен не тем флагом: %v", spec.Args)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("брокер не поднялся")
	}
	if s.brokerFor("d1") == nil {
		t.Fatal("брокер не зарегистрирован для загрузки")
	}
}

func TestHandleDownloadStartedStaysOutWhenNotAsked(t *testing.T) {
	no := false
	cases := []struct {
		name  string
		mutir func(*download.Download)
	}{
		{"права заранее не просили", func(d *download.Download) { d.Origin.ElevateAhead = false }},
		{"автоустановка отключена для этой загрузки", func(d *download.Download) { d.Origin.AutoInstall = &no }},
		{"загрузка не про релиз", func(d *download.Download) { d.Origin.Purpose = download.PurposeUpdate }},
		{"в загрузке нет установщика", func(d *download.Download) {
			d.Files = []download.FileState{{Path: "Game/Game.bin", Selected: true}}
		}},
		{"установщик не выбран", func(d *download.Download) {
			d.Files = []download.FileState{{Path: "setup.exe", Selected: false}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _ := newTestService(t)
			started, _ := fakeElevation(t)
			d := startedDownload(t, s)
			tc.mutir(&d)

			s.HandleDownloadStarted(d)

			select {
			case spec := <-started:
				t.Fatalf("брокер поднялся зря: %v", spec.Args)
			case <-time.After(50 * time.Millisecond):
			}
			if s.brokerFor("d1") != nil {
				t.Fatal("брокер зарегистрирован зря")
			}
		})
	}
}

func TestHandleDownloadStartedSkipsWithoutElevation(t *testing.T) {
	s, _, _ := newTestService(t)
	started, _ := fakeElevation(t)
	elevationAvailable = func() bool { return false }

	s.HandleDownloadStarted(startedDownload(t, s))

	select {
	case <-started:
		t.Fatal("брокер поднялся там, где повышения прав не бывает")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStartBrokerPinsToTheDownload(t *testing.T) {
	s, _, _ := newTestService(t)
	fakeElevation(t)
	d := startedDownload(t, s)

	s.HandleDownloadStarted(d)
	handoff := s.brokerFor("d1")
	if handoff == nil {
		t.Fatal("брокер не зарегистрирован")
	}

	pin, err := readBrokerPin(brokerPinPath(handoff.Dir))
	if err != nil {
		t.Fatalf("read pin: %v", err)
	}
	if pin.ContentRoot != d.Destination {
		t.Errorf("ContentRoot = %q, ожидался %q", pin.ContentRoot, d.Destination)
	}
	if pin.LibraryRoot != s.config().GamesPath {
		t.Errorf("LibraryRoot = %q, ожидался %q", pin.LibraryRoot, s.config().GamesPath)
	}
	if pin.StateDir != s.store.dir {
		t.Errorf("StateDir = %q, ожидался %q", pin.StateDir, s.store.dir)
	}
}

// Мёртвый брокер обязан перестать выдаваться: иначе установка положит задание
// в каталог, который никто не читает, и будет ждать его вечно.
func TestBrokerForForgetsADeadBroker(t *testing.T) {
	s, _, _ := newTestService(t)
	_, procs := fakeElevation(t)

	s.HandleDownloadStarted(startedDownload(t, s))
	if s.brokerFor("d1") == nil {
		t.Fatal("брокер не зарегистрирован")
	}
	(<-procs).close()

	waitFor(t, "broker to be reported dead", func() bool { return s.brokerFor("d1") == nil })
}

func TestDropBrokerAsksItToExit(t *testing.T) {
	s, _, _ := newTestService(t)
	fakeElevation(t)

	s.HandleDownloadStarted(startedDownload(t, s))
	handoff := s.brokerFor("d1")
	if handoff == nil {
		t.Fatal("брокер не зарегистрирован")
	}
	dir := handoff.Dir

	s.DropBroker("d1")

	if _, err := os.Stat(brokerAbortPath(dir)); err != nil {
		t.Fatalf("метка выхода не выставлена: %v", err)
	}
	if s.brokerFor("d1") != nil {
		t.Fatal("снятый брокер всё ещё выдаётся")
	}
}

// Задание уходит живому брокеру, а не новому процессу: ради этого его и
// поднимали заранее.
func TestHandOffPrefersTheLiveBroker(t *testing.T) {
	dir := t.TempDir()
	gone := make(chan struct{})
	spec := runSpec{
		ID:        "i1",
		StatePath: filepath.Join(dir, "state.json"),
		Broker:    &brokerHandoff{Key: brokerTestPrivate, Dir: dir, Gone: gone},
	}
	ws := workerSpec{ID: "i1", InstallerPath: filepath.Join(dir, "setup.exe")}
	if err := os.WriteFile(ws.InstallerPath, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}

	spawned := false
	old := startElevatedWorker
	startElevatedWorker = func(runSpec) (workerHandle, error) {
		spawned = true
		return newFakeProc(), nil
	}
	t.Cleanup(func() { startElevatedWorker = old })

	exited, cleanup, _, err := handOffToWorker(context.Background(), spec, ws, filepath.Join(dir, "worker-spec.json"))
	if err != nil {
		t.Fatalf("handOffToWorker: %v", err)
	}
	defer cleanup()
	if spawned {
		t.Fatal("поднят новый воркер, хотя брокер был жив")
	}

	handed, err := readWorkerSpec(brokerSpecPath(dir))
	if err != nil {
		t.Fatalf("брокер не получил задание: %v", err)
	}
	if handed.InstallerPath != ws.InstallerPath {
		t.Errorf("InstallerPath = %q, ожидался %q", handed.InstallerPath, ws.InstallerPath)
	}

	close(gone)
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("смерть брокера не дошла до цикла ожидания")
	}
}

// TestAskBrokerToExitRetriesOnTransientFailure закрывает находку 2 (вторая
// половина): до фикса askBrokerToExit ничего не возвращал, и одиночный отказ
// записи (антивирус держит хэндл, EACCES) молча оставлял брокера с правами
// администратора не узнавшим, что его просили выйти — все четыре вызывающих
// в этом файле считали неудачную запись успехом.
func TestAskBrokerToExitRetriesOnTransientFailure(t *testing.T) {
	dir := t.TempDir()
	restoreInterval, restoreWindow := brokerAskRetryInterval, brokerAskRetryWindow
	brokerAskRetryInterval = 5 * time.Millisecond
	brokerAskRetryWindow = 2 * time.Second
	t.Cleanup(func() { brokerAskRetryInterval, brokerAskRetryWindow = restoreInterval, restoreWindow })

	abortPath := brokerAbortPath(dir)
	if err := os.MkdirAll(abortPath, 0o755); err != nil {
		t.Fatalf("seed blocking directory: %v", err)
	}
	go func() {
		<-time.After(30 * time.Millisecond)
		if err := os.Remove(abortPath); err != nil {
			t.Errorf("remove blocking directory: %v", err)
		}
	}()

	if err := askBrokerToExit(dir); err != nil {
		t.Fatalf("askBrokerToExit = %v, want the transient failure to be retried away", err)
	}
	if _, err := os.Stat(abortPath); err != nil {
		t.Fatalf("abort marker not written after retry: %v", err)
	}
}

// TestAskBrokerToExitReportsPersistentFailure — вторая половина: отказ,
// который не проходит за отведённое время, остаётся ошибкой и доходит до
// вызывающего, а не тонет молча.
func TestAskBrokerToExitReportsPersistentFailure(t *testing.T) {
	dir := t.TempDir()
	restoreInterval, restoreWindow := brokerAskRetryInterval, brokerAskRetryWindow
	brokerAskRetryInterval = 5 * time.Millisecond
	brokerAskRetryWindow = 30 * time.Millisecond
	t.Cleanup(func() { brokerAskRetryInterval, brokerAskRetryWindow = restoreInterval, restoreWindow })

	abortPath := brokerAbortPath(dir)
	if err := os.MkdirAll(abortPath, 0o755); err != nil {
		t.Fatalf("seed blocking directory: %v", err)
	}
	// Никогда не снимается: запись остаётся заблокированной всё окно ретрая.

	if err := askBrokerToExit(dir); err == nil {
		t.Fatal("askBrokerToExit вернул nil для записи, которая ни разу не удалась")
	}
}

// TestServiceShutdownWaitsForBrokerProcessToActuallyExit закрывает находку 5:
// go func(){ defer close(b.gone); defer b.proc.close(); b.proc.wait() }() в
// tendBroker не проходила через s.wg. Внешняя tendBroker снимает свой
// wg.Done() по таймауту brokerStopWait независимо от того, вышел ли
// настоящий процесс брокера, и до фикса ServiceShutdown возвращался сразу за
// ней, даже если внутренняя горутина всё ещё висела на wait() — процесс с
// правами администратора оставался никем не отслеживаемым.
func TestServiceShutdownWaitsForBrokerProcessToActuallyExit(t *testing.T) {
	s := mustServiceAt(t, t.TempDir())
	s.settings = newTestSettings(t)
	s.downloads = newFakeDownloads()
	s.library = &fakeRegistrar{}
	if err := s.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("startup: %v", err)
	}
	_, procs := fakeElevation(t)

	s.HandleDownloadStarted(startedDownload(t, s))
	var proc *fakeProc
	select {
	case proc = <-procs:
	case <-time.After(5 * time.Second):
		t.Fatal("брокер не поднялся")
	}
	if s.brokerFor("d1") == nil {
		t.Fatal("брокер не зарегистрирован")
	}

	done := make(chan error, 1)
	go func() { done <- s.ServiceShutdown() }()

	select {
	case <-done:
		t.Fatal("ServiceShutdown вернулся, хотя процесс брокера ещё жив и никем не отслеживается")
	case <-time.After(200 * time.Millisecond):
	}

	proc.close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServiceShutdown не вернулся после закрытия процесса брокера")
	}
}
