package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"typhon/internal/history"
	"typhon/internal/library"
	"typhon/internal/settings"
	"typhon/internal/usagestats"
)

func (s *Service) run(ctx context.Context, id string) {
	item, ok := s.snapshot(id)
	if !ok {
		return
	}
	var err error
	switch {
	case item.Type == TypePortable:
		err = s.runPortable(ctx, id, item)
	case archived(item.Type):
		err = s.runArchive(ctx, id, item)
	case external(item.Type):
		err = s.runInstaller(ctx, id, item)
	default:
		err = errUnknownType
	}
	if err != nil {
		s.fail(id, err)
	}
	if external(item.Type) {
		s.releaseWorkerFiles(id, err)
	}
}

func (s *Service) runPortable(ctx context.Context, id string, item Installation) error {
	if err := s.setStatus(id, StatusPreparing); err != nil {
		return err
	}
	partial := item.Destination + partialSuffix
	if err := os.RemoveAll(partial); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.setStatus(id, StatusInstalling); err != nil {
		return err
	}

	report := func(p Progress) { s.updateProgress(id, p) }
	var err error
	// Keep the source until both the destination and library entry are committed.
	err = CopyDirVerified(ctx, item.ContentRoot, partial, report)
	if err == nil {
		err = s.commit(ctx, partial, item.Destination)
	}
	if err != nil {
		s.cleanupPartial(partial)
		return err
	}
	if err := s.finalize(ctx, id); err != nil {
		return err
	}
	if item.Mode == ModeMove {
		if err := removeInstalledSource(item.ContentRoot); err != nil {
			slog.Warn("installed successfully, source cleanup incomplete", "path", item.ContentRoot, "error", err)
		}
	}
	return nil
}

func (s *Service) runArchive(ctx context.Context, id string, item Installation) error {
	if err := s.setStatus(id, StatusPreparing); err != nil {
		return err
	}
	partial := item.Destination + partialSuffix
	if err := os.RemoveAll(partial); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.setStatus(id, StatusExtracting); err != nil {
		return err
	}

	err := ExtractArchive(ctx, item.ArchivePath, partial, func(p Progress) { s.updateProgress(id, p) })
	if err == nil {
		err = s.commitExtracted(ctx, partial, item.Destination)
	}
	if err != nil {
		s.cleanupPartial(partial)
		return err
	}
	return s.finalize(ctx, id)
}

func (s *Service) runInstaller(ctx context.Context, id string, item Installation) error {
	// Брокер поднимается заранее (HandleDownloadStarted), пока лаунчер ещё не
	// знает, какой веткой пойдёт эта установка: задание ему отдаёт и тихая, и
	// интерактивная ветка (brokerFor). Освобождать его нужно на любом выходе
	// из этой функции, а не только из одной ветки — иначе репак, для которого
	// брокер подняли заранее, держит процесс с правами администратора до
	// закрытия лаунчера.
	defer s.DropBroker(item.DownloadID)
	if err := s.setStatus(id, StatusPreparing); err != nil {
		return err
	}
	cfg := s.config()
	roots := s.installRoots()
	before, err := takeSnapshot(roots)
	if err != nil {
		return err
	}
	beforeEntries, err := s.readEntries()
	if err != nil {
		return err
	}
	shell := s.shellBaseline(ctx, id)
	if err := ctx.Err(); err != nil {
		return err
	}
	if runsSilently(item) {
		if err := s.rememberInstallerDestination(id, item.Destination); err != nil {
			return err
		}
		return s.runSilent(ctx, id, item, roots, before, beforeEntries, shell)
	}
	if item.Unattended {
		return errNeedsUser
	}
	if err := s.setStatus(id, StatusInstalling); err != nil {
		return err
	}

	engine := item.Engine
	if item.Type == TypeMsiInstaller {
		engine = EngineMsi
	}
	handoff := s.brokerFor(item.DownloadID)
	for _, installer := range installerChain(item) {
		spec, err := interactiveRunSpec(engine, installer, item.WorkingDir)
		if err != nil {
			return err
		}
		spec.ID = item.ID
		spec.Engine = engine
		spec.InstallerPath = installer
		// Shell не передаётся: каталог игры мастер выбирает пользователь, и до
		// конца установки он неизвестен, а воркеру цель уборки ярлыков нужна в
		// задании. Ярлыки после интерактивной установки убирает лаунчер.
		spec = s.bindWorker(spec, handoff, nil)

		s.setExternal(id, true)
		code, err := s.runner.run(ctx, spec)
		s.setExternal(id, false)
		if err != nil {
			return err
		}
		if exitErr := exitError(item.Engine, code); exitErr != nil {
			slog.Error("installer exit code", "id", id, "path", installer, "code", code)
			return exitErr
		}
	}

	after, err := takeSnapshot(roots)
	if err != nil {
		return err
	}
	dirs := shallowest(append(diffSnapshot(before, after), s.entryDirs(id, item, roots, beforeEntries)...))
	candidates, err := gather(ctx, dirs, item.Name)
	if err != nil {
		return err
	}
	dest := pickInstallDir(dirs, candidates)
	if dest != "" {
		if err := s.setDestination(id, dest); err != nil {
			return err
		}
	}
	if err := s.setRemoval(id, dest, before.withUnseen(dest), beforeEntries, item.Name); err != nil {
		return err
	}
	s.dropShortcuts(ctx, id, shell, dest, cfg.InstallSkipShortcuts, nil)
	return s.waitForUser(id, candidates)
}

func (s *Service) runSilent(ctx context.Context, id string, item Installation, roots []string, before fsSnapshot, beforeEntries map[string]uninstallEntry, shell shellSnapshot) error {
	logPath := s.installerLogPath(id)
	opts := installOptionsFrom(s.config())
	chain := installerChain(item)
	// DropBroker освобождается один раз для всей установки в runInstaller —
	// дальше по цепочке установщиков этот же брокер ещё нужен.
	handoff := s.brokerFor(item.DownloadID)
	shared, err := newShellHandoff(shell, opts.SkipShortcuts)
	if err != nil {
		slog.Warn("shortcut cleanup stays with the launcher", "id", id, "error", err)
	}
	steps, err := s.chainSteps(item, chain, 0, logPath, opts, handoff, shared)
	if err != nil {
		return err
	}
	workers := make([]*shellHandoff, 0, len(steps))
	for _, step := range steps {
		workers = append(workers, step.spec.Shell)
	}
	if err := s.setStatus(id, StatusInstalling); err != nil {
		return err
	}

	stop := s.trackInstallSize(ctx, id, item.Destination, item.BytesTotal, logPath, opts.VerifyRepack)
	runErr := s.runSilentChain(ctx, id, item, steps, logPath)
	stop()
	if runErr != nil {
		s.discardSilent(item, before, runErr)
		return runErr
	}

	dropInstallerLog(logPath)

	dest, err := s.silentDestination(ctx, id, item, roots, before, beforeEntries)
	if err != nil {
		s.discardSilent(item, before, err)
		return err
	}
	seen := before
	if dest != item.Destination {
		seen = before.withUnseen(dest)
	}
	if err := s.setRemoval(id, dest, seen, beforeEntries, item.Name); err != nil {
		return err
	}
	s.dropShortcuts(ctx, id, shell, dest, opts.SkipShortcuts, workers)
	return s.finalize(ctx, id)
}

type chainStep struct {
	number int
	path   string
	spec   runSpec
}

// chainSteps строит задания для установщиков chain начиная с индекса from:
// общий кусок для первого запуска (runSilent) и для продолжения цепочки после
// перезапуска лаунчера (finishResumed), чтобы пути состояния, брокер и ярлыки
// задавались в одном месте.
func (s *Service) chainSteps(item Installation, chain []string, from int, logPath string, opts installOptions, handoff *brokerHandoff, shared *shellHandoff) ([]chainStep, error) {
	steps := make([]chainStep, 0, len(chain)-from)
	for i := from; i < len(chain); i++ {
		spec, err := silentSpec(item, chain[i], logPath, opts)
		if err != nil {
			return nil, err
		}
		spec = s.bindWorker(spec, handoff, shared.forInstaller())
		steps = append(steps, chainStep{number: i + 1, path: chain[i], spec: spec})
	}
	return steps, nil
}

// bindWorker привязывает задание к повышенному воркеру: файлы состояния,
// отмены и разведки, брокер и уборка ярлыков. Единственное место, где они
// задаются, — и для тихой цепочки, и для ручной установки (runInstaller),
// иначе запуск, которому понадобился UAC, не находит, через что говорить с
// воркером (errWorkerStatePath).
func (s *Service) bindWorker(spec runSpec, handoff *brokerHandoff, shell *shellHandoff) runSpec {
	spec.StatePath = s.workerStatePath(spec.ID)
	spec.InfPath = s.workerInfPath(spec.ID)
	spec.CancelPath = s.workerCancelPath(spec.ID)
	spec.Broker = handoff
	spec.Shell = shell
	return spec
}

// runsSilently — единственное правило, по которому установка идёт тихой
// веткой: им решают и runInstaller, и ServiceStartup, чтобы воркер ручной
// установки после перезапуска не приняли за тихий.
func runsSilently(item Installation) bool {
	return item.Silent && item.Destination != ""
}

// beginChainStep записывает номер установщика цепочки до его запуска: если
// лаунчер умрёт посреди шага, после перезапуска только по этому номеру видно,
// остались ли за ним ещё установщики. Файл состояния воркера один на всю
// цепочку, поэтому результат предыдущего шага убирается раньше номера: иначе
// смерть между двумя записями оставила бы чужой Done рядом с новым номером.
func (s *Service) beginChainStep(id string, number int) error {
	if path := s.workerStatePath(id); path != "" {
		if err := os.Remove(path); err != nil && !alreadyGone(err) {
			return fmt.Errorf("remove worker state %s: %w", path, err)
		}
	}
	s.mu.Lock()
	item := s.findLocked(id)
	if item == nil {
		s.mu.Unlock()
		return errNotFound
	}
	if !active(item.Status) {
		s.mu.Unlock()
		return errUnavailable
	}
	prev := item.ChainStep
	item.ChainStep = number
	if err := s.persistLocked(); err != nil {
		item.ChainStep = prev
		s.mu.Unlock()
		return wrapPersistError(err)
	}
	s.mu.Unlock()
	return nil
}

// runSilentChain прогоняет установщики набора по очереди: дополнение GOG ставится
// только поверх уже установленной игры, поэтому порядок из плана обязателен, а
// первая же неудача останавливает цепочку.
func (s *Service) runSilentChain(ctx context.Context, id string, item Installation, steps []chainStep, logPath string) error {
	track := len(installerChain(item)) > 1
	for _, step := range steps {
		if track {
			if err := s.beginChainStep(id, step.number); err != nil {
				return err
			}
		}
		dropInstallerLog(logPath)
		code, err := s.runner.run(ctx, step.spec)
		if err != nil {
			return err
		}
		done, logErr := installerFinished(item.Engine, code, logPath)
		if logErr != nil {
			slog.Warn("read installer log", "id", id, "path", logPath, "error", logErr)
		}
		if !done {
			slog.Error("silent installer failed", "id", id, "engine", string(item.Engine),
				"path", step.path, "code", code, "log", installerLogTail(logPath))
			return installerFailure(item.Engine, code, logPath)
		}
		if exitErr := exitError(item.Engine, code); exitErr != nil {
			// Установщики GOG падают при завершении уже после того, как файлы
			// разложены: свой лог они при этом закрывают отметкой об успехе.
			slog.Warn("installer crashed after finishing", "id", id, "engine", string(item.Engine),
				"path", step.path, "code", code)
		}
	}
	return nil
}

var errResumedCancelled = errors.New("отмена запрошена до продолжения цепочки установщиков")

// resumeChain вызывается, когда воркер подтвердил успех текущего установщика
// уже после перезапуска лаунчера. Цепочка из одного установщика на этом
// закончена. В длинной номер текущего шага читается из записи: нет номера
// (старая запись) или он вне цепочки значит, что нельзя сказать, чем кончилась
// установка, и объявлять её готовой нельзя. Остаток цепочки идёт тем же
// runSilentChain, что и обычный запуск, но без снимков до установки: брокера
// и общих ярлыков у такого продолжения нет.
func (s *Service) resumeChain(ctx context.Context, id string, item Installation, logPath string) error {
	chain := installerChain(item)
	if len(chain) <= 1 {
		return nil
	}
	if item.ChainStep < 1 || item.ChainStep > len(chain) {
		return fmt.Errorf("%w: в цепочке %d установщиков, записан шаг %d", errChainStepUnknown, len(chain), item.ChainStep)
	}
	if item.ChainStep == len(chain) {
		return nil
	}
	next := item.ChainStep
	for i := next; i < len(chain); i++ {
		info, err := os.Stat(chain[i])
		if err != nil {
			return fmt.Errorf("установщик %d из %d, %s: %w: %w", i+1, len(chain), chain[i], errChainInstallerMissing, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("установщик %d из %d, %s: %w: не обычный файл", i+1, len(chain), chain[i], errChainInstallerMissing)
		}
	}
	if workerCancelRequested(s.workerCancelPath(id)) {
		return errResumedCancelled
	}
	opts := installOptionsFrom(s.config())
	steps, err := s.chainSteps(item, chain, next, logPath, opts, s.brokerFor(item.DownloadID), nil)
	if err != nil {
		return fmt.Errorf("установщик %d из %d: %w", next+1, len(chain), err)
	}
	slog.Warn("continuing installer chain after launcher restart", "id", id, "done", item.ChainStep, "total", len(chain))
	stop := s.trackInstallSize(ctx, id, item.Destination, item.BytesTotal, logPath, opts.VerifyRepack)
	err = s.runSilentChain(ctx, id, item, steps, logPath)
	stop()
	if err != nil {
		failedAt := next + 1
		if cur, ok := s.snapshot(id); ok && cur.ChainStep > failedAt {
			failedAt = cur.ChainStep
		}
		return fmt.Errorf("установщик %d из %d: %w", failedAt, len(chain), err)
	}
	return nil
}

func installerChain(item Installation) []string {
	chain := make([]string, 0, len(item.ExtraInstallers)+1)
	if item.InstallerPath != "" {
		chain = append(chain, item.InstallerPath)
	}
	return append(chain, item.ExtraInstallers...)
}

func installOptionsFrom(cfg settings.Settings) installOptions {
	return installOptions{SkipShortcuts: cfg.InstallSkipShortcuts, SkipExtras: cfg.InstallSkipExtras, VerifyRepack: cfg.InstallVerifyRepack}
}

// Снимок ярлыков берётся до запуска установщика: без него не отличить ярлык,
// созданный установкой, от ярлыка пользователя, поэтому ошибка обхода отменяет
// уборку целиком, а не разрешает удалять наугад. Берётся всегда, а не только
// при InstallSkipShortcuts: ярлык сайта репака убирается независимо от неё.
func (s *Service) shellBaseline(ctx context.Context, id string) shellSnapshot {
	roots, err := shortcutRootsFn()
	if err != nil {
		slog.Error("resolve shortcut folders", "id", id, "error", err)
		return shellSnapshot{}
	}
	snap, err := takeShellSnapshot(ctx, roots)
	if err != nil {
		slog.Error("scan shortcut folders", "id", id, "error", err)
		return shellSnapshot{}
	}
	return snap
}

// Ярлыки, созданные установщиком под UAC в общих каталогах, лаунчер удалить не
// может: он работает без прав администратора. Если после каждого установщика
// цепочки их убрал повышенный воркер, лаунчер чистит только свои каталоги;
// общие каталоги берутся из того же задания, что ушло воркеру, и заново не
// определяются. Это не повод считать установку неудачной, поэтому ошибка
// только логируется: канала для предупреждений у задания установки нет.
func (s *Service) dropShortcuts(ctx context.Context, id string, before shellSnapshot, dest string, game bool, workers []*shellHandoff) {
	if !before.taken {
		return
	}
	logShellReports(id, workerReports(workers))
	if shared := delegatedRoots(workers); len(shared) > 0 {
		before = before.without(shared)
	}
	removed, err := cleanShellShortcuts(ctx, before, dest, game)
	if err != nil {
		slog.Warn("remove installer shortcuts", "id", id, "dest", dest, "error", err)
	}
	if len(removed) > 0 {
		slog.Info("installer shortcuts removed", "id", id, "count", len(removed), "paths", removed)
	}
}

// silentDestination доверяет заданному каталогу только после того, как убедился,
// что установщик действительно в него писал: часть установщиков игнорирует
// ключ каталога и ставит игру по своему пути, и тогда его надо найти по снимку.
func (s *Service) silentDestination(ctx context.Context, id string, item Installation, roots []string, before fsSnapshot, beforeEntries map[string]uninstallEntry) (string, error) {
	empty, err := dirEmpty(item.Destination)
	if err != nil {
		return "", err
	}
	if !empty {
		return item.Destination, nil
	}
	after, err := takeSnapshot(roots)
	if err != nil {
		return "", err
	}
	dirs := shallowest(append(diffSnapshot(before, after), s.entryDirs(id, item, roots, beforeEntries)...))
	candidates, err := gather(ctx, dirs, item.Name)
	if err != nil {
		return "", err
	}
	found := pickInstallDir(dirs, candidates)
	if found == "" {
		return "", errInstallerNoOutput
	}
	found, err = normalizeRoot(found)
	if err != nil {
		return "", err
	}
	if err := os.Remove(item.Destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("remove empty install dir", "path", item.Destination, "error", err)
	}
	slog.Warn("installer ignored target directory", "id", id, "want", item.Destination, "got", found)
	if err := s.forceDestination(id, found); err != nil {
		return "", err
	}
	return found, nil
}

// discardSilent удаляет каталог, в который писала неудавшаяся тихая
// установка, но только если процесс, который туда писал, точно не жив:
// errInstallerNotConfirmedStopped значит ровно обратное — установщик под
// UAC не подтвердил остановку, и RemoveAll на живого писателя — гонка на
// единственной копии данных (инвариант 9).
func (s *Service) discardSilent(item Installation, before fsSnapshot, cause error) {
	if item.Destination == "" || s.isClosing() {
		return
	}
	if errors.Is(cause, errInstallerNotConfirmedStopped) {
		slog.Warn("keep install dir, installer not confirmed stopped", "path", item.Destination, "error", cause)
		return
	}
	if _, existed := before.dirs[item.Destination]; existed {
		return
	}
	if err := os.RemoveAll(item.Destination); err != nil {
		slog.Warn("remove failed install dir", "path", item.Destination, "error", err)
	}
}

func (s *Service) trackInstallSize(ctx context.Context, id, dir string, total int64, logPath string, verifyRepack bool) func() {
	ctx, cancel := context.WithCancel(ctx)
	s.wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer s.wg.Done()
		defer close(done)
		ticker := time.NewTicker(installPollInterval)
		defer ticker.Stop()
		verifying := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				if verifyRepack && !verifying && installerLogVerifying(logPath) {
					if err := s.setInstallerVerifying(id); err == nil {
						verifying = true
					}
				}
				size, err := DirSize(ctx, dir)
				if err != nil {
					// Каталог меняется под обходом (установщик создаёт и удаляет
					// файлы): пропущенный замер — это тик без обновления, а не
					// сбой установки, следующий тик замерит заново.
					slog.Debug("measure install size", "id", id, "dir", dir, "error", err)
					continue
				}
				s.updateProgress(id, Progress{BytesDone: size, BytesTotal: total})
			}
		}
	}()
	return func() { cancel(); <-done }
}

func silentSpec(item Installation, installer, logPath string, opts installOptions) (runSpec, error) {
	plan, err := silentArgs(item.Engine, installer, item.Destination, logPath, opts)
	if err != nil {
		return runSpec{}, err
	}
	path := installer
	if item.Engine == EngineMsi {
		msiexec, err := systemExecutable("msiexec.exe")
		if err != nil {
			return runSpec{}, err
		}
		path = msiexec
	}
	return runSpec{
		Path: path, Args: plan.Args, Dir: item.WorkingDir, CmdLine: plan.CmdLine, Tail: plan.Tail, Background: true, Hidden: true,
		ID: item.ID, Engine: item.Engine, InstallerPath: installer, Destination: item.Destination, LogPath: logPath, Options: opts,
	}, nil
}

// interactiveRunSpec — запуск установщика без ключей тишины: общий для
// лаунчера (runInstaller) и повышенного воркера (mainRunSpec), чтобы то, что
// видит пользователь, не зависело от того, понадобились ли права администратора.
func interactiveRunSpec(engine Engine, installer, dir string) (runSpec, error) {
	if engine != EngineMsi {
		return runSpec{Path: installer, Dir: dir, Interactive: true}, nil
	}
	msiexec, err := systemExecutable("msiexec.exe")
	if err != nil {
		return runSpec{}, err
	}
	return runSpec{Path: msiexec, Args: []string{"/i", installer}, Dir: dir, Interactive: true}, nil
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dir %s: %w", dir, err)
	}
	return len(entries) == 0, nil
}

func dropInstallerLog(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("remove installer log", "path", path, "error", err)
	}
}

// installerFinished — единственное правило, по которому установщик считается
// отработавшим: им решает и лаунчер (runSilentChain), и повышенный воркер,
// прежде чем убирать ярлыки (workerShellCleanup).
func installerFinished(engine Engine, code int, logPath string) (bool, error) {
	if exitError(engine, code) == nil {
		return true, nil
	}
	return installerLogSucceeded(engine, logPath)
}

// installerLogSucceeded отвечает на вопрос, разложил ли установщик файлы, когда
// код возврата говорит об обратном: Inno закрывает свой лог отметкой об успехе
// до кода возврата, и падение на выходе не отменяет уже сделанную установку.
func installerLogSucceeded(engine Engine, path string) (bool, error) {
	return innoLogContains(engine, path, innoSuccessMarker)
}

// installerFailure превращает неуспех тихой установки в ошибку: код возврата
// Inno не отличает «установщик не умеет тишину» от обычного сбоя, это видно
// только по логу, где мастер отказался переходить на следующую страницу.
// Нечитаемый лог не отменяет сам неуспех, поэтому возвращается ошибка по коду.
func installerFailure(engine Engine, code int, logPath string) error {
	exitErr := exitError(engine, code)
	if exitErr == nil {
		return nil
	}
	refused, err := innoLogContains(engine, logPath, innoWizardRefusedMarker)
	if err != nil {
		slog.Warn("read installer log", "path", logPath, "error", err)
		return exitErr
	}
	if refused {
		return exitCodeError(errInstallerNeedsInteractive, engine, code)
	}
	return exitErr
}

func innoLogContains(engine Engine, path, marker string) (bool, error) {
	if engine != EngineInno || path == "" {
		return false, nil
	}
	data, err := readLogTail(path, installerLogScanLimit)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(decodeLogText(data), marker), nil
}

func installerLogTail(path string) string {
	if path == "" {
		return ""
	}
	data, err := readLogTail(path, installerLogTailLimit)
	if err != nil {
		return "лог недоступен: " + err.Error()
	}
	return strings.TrimSpace(decodeLogText(data))
}

func readLogTail(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close installer log", "path", path, "error", err)
		}
	}()
	if info.Size() > limit {
		if _, err := f.Seek(info.Size()-limit, io.SeekStart); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(f, limit))
}

// Inno пишет лог в UTF-16LE, а на диске он может оказаться и в UTF-8: нулевые
// байты убираем, чтобы обе кодировки читались одним поиском по подстроке.
func decodeLogText(data []byte) string {
	return strings.ReplaceAll(string(data), "\x00", "")
}

// rememberInstallerDestination persists an empty target before the installer can
// write into it. Never claim an existing nonempty directory on a legacy retry.
func (s *Service) rememberInstallerDestination(id, destination string) error {
	if !destAvailable(destination) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || item.OwnedDestination == destination {
		return nil
	}
	prev := item.OwnedDestination
	item.OwnedDestination = destination
	if err := s.persistLocked(); err != nil {
		item.OwnedDestination = prev
		return wrapPersistError(err)
	}
	return nil
}

// setRemoval выясняет, чем игру потом удалять: свежая запись в ветке Uninstall
// даёт деинсталлятор, а отсутствие каталога в снимке до установки — право
// удалить каталог целиком. Ошибка чтения реестра не превращается в «удалять
// нечем»: она помечается UninstallUnknown, и UI предложит системный апплет.
func (s *Service) setRemoval(id, destination string, before fsSnapshot, beforeEntries map[string]uninstallEntry, name string) error {
	owned := false
	if destination != "" {
		_, existed := before.dirs[destination]
		owned = !existed
	}
	uninstall, unknown := library.Uninstall{}, false
	afterEntries, err := s.readEntries()
	if err != nil {
		slog.Error("read uninstall entries", "id", id, "error", err)
		unknown = true
	} else if picked, ok := pickUninstall(beforeEntries, afterEntries, destination, name); ok {
		uninstall = picked
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil {
		return nil
	}
	prevOwned, prevUninstall, prevUnknown := item.Owned, item.Uninstall, item.UninstallUnknown
	item.Owned = owned || (destination != "" && samePath(item.OwnedDestination, destination))
	item.Uninstall = uninstall
	item.UninstallUnknown = unknown
	if err := s.persistLocked(); err != nil {
		item.Owned = prevOwned
		item.Uninstall = prevUninstall
		item.UninstallUnknown = prevUnknown
		return wrapPersistError(err)
	}
	return nil
}

func (s *Service) commitExtracted(ctx context.Context, partial, destination string) error {
	root, err := normalizeRoot(partial)
	if err != nil {
		return err
	}
	if root == partial {
		return s.commit(ctx, partial, destination)
	}
	if err := s.commit(ctx, root, destination); err != nil {
		return err
	}
	s.cleanupPartial(partial)
	return nil
}

func (s *Service) commit(ctx context.Context, partial, destination string) error {
	if entries, err := os.ReadDir(destination); err == nil && len(entries) == 0 {
		// Best-effort: this only clears the way for the Rename below. If it
		// fails, Rename fails too and falls back to MoveDir, which handles a
		// non-empty (or still-present) destination on its own.
		if err := os.Remove(destination); err != nil {
			slog.Warn("remove empty destination before rename", "destination", destination, "error", err)
		}
	}
	if err := os.Rename(partial, destination); err == nil {
		return nil
	}
	return MoveDir(ctx, partial, destination, nil)
}

func (s *Service) cleanupPartial(partial string) {
	if partial == "" || s.isClosing() {
		return
	}
	if err := os.RemoveAll(partial); err != nil {
		slog.Warn("remove partial install", "path", partial, "error", err)
	}
}

func (s *Service) finalize(ctx context.Context, id string) error {
	if err := s.setStatus(id, StatusVerifying); err != nil {
		return err
	}
	item, ok := s.snapshot(id)
	if !ok {
		return errNotFound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if item.Executable == "" {
		candidates, err := FindExecutables(ctx, item.Destination, item.Name)
		if err != nil {
			return err
		}
		switch {
		case HighConfidence(candidates):
			if err := s.setExecutable(id, candidates[0].Path, candidates); err != nil {
				return err
			}
		case item.Unattended:
			executable := ""
			if len(candidates) > 0 {
				executable = candidates[0].Path
			}
			if err := s.setExecutable(id, executable, candidates); err != nil {
				return err
			}
		default:
			return s.waitForUser(id, candidates)
		}
	}
	return s.complete(ctx, id)
}

func (s *Service) complete(ctx context.Context, id string) error {
	item, ok := s.snapshot(id)
	if !ok {
		return errNotFound
	}
	cfg := s.config()
	if cfg.VerifyAfterInstall {
		if err := verifyInstall(item); err != nil {
			return err
		}
	}
	version, source := detectVersion(item)
	var game library.Game
	if !item.SkipRegister {
		registered, err := s.register(item, version, source)
		if err != nil {
			return err
		}
		game = registered
		// Окружение запуска — то же удобство поверх установки, что и ярлык:
		// если бутыль не завёлся, игра всё равно установлена, а попытка
		// повторится при первом запуске.
		if err := s.prepareRuntime(ctx, item.Destination, game.Executable); err != nil {
			slog.Warn("prepare game runtime", "id", game.ID, "error", err)
		}
		if cfg.DesktopShortcuts {
			// Ярлык — удобство поверх установки, а не её часть: рабочий
			// стол может быть недоступен, и объявлять из-за этого
			// установленную игру неустановленной нельзя. Кнопка «создать
			// ярлык» на странице игры остаётся ручным повтором.
			if err := s.library.CreateShortcut(game.ID); err != nil {
				slog.Warn("create desktop shortcut", "id", game.ID, "error", err)
			}
		}
	}

	s.mu.Lock()
	stored := s.findLocked(id)
	if stored == nil {
		s.mu.Unlock()
		return errNotFound
	}
	prev := snapshotOf(stored)
	now := time.Now()
	stored.Status = StatusCompleted
	stored.GameID = game.ID
	stored.DetectedVersion = version
	stored.VersionSource = source
	stored.Progress = 1
	stored.CurrentFile = ""
	stored.Error = ""
	stored.CompletedAt = &now
	if err := s.persistLocked(); err != nil {
		*stored = prev
		s.mu.Unlock()
		return wrapPersistError(err)
	}
	snap := snapshotOf(stored)
	s.mu.Unlock()

	slog.Info("install completed", "id", id, "name", snap.Name, "game", game.ID, "version", version)
	emit(eventCompleted, snap)
	duration := time.Duration(0)
	if snap.CompletedAt != nil {
		duration = snap.CompletedAt.Sub(snap.StartedAt)
	}
	s.recordUsage(usagestats.Event{
		Type:      usagestats.TypeInstallCompleted,
		Timestamp: time.Now(),
		Properties: usagestats.Properties{
			GameID:          snap.Origin.GameID,
			InstallerType:   installerType(snap.Type),
			DurationSeconds: usageDurationSeconds(duration),
		},
	})
	emit(eventUpdated, snap)
	if !item.SkipRegister {
		s.applyCleanup(cfg, snap.DownloadID)
	}
	s.notifyFinished(snap)
	if s.historyRecorder != nil {
		if err := s.historyRecorder(history.Record{
			Kind:      history.KindInstalled,
			GameID:    game.ID,
			Title:     snap.Name,
			ToVersion: version,
			RefID:     id,
		}); err != nil {
			// complete() runs at the end of a background install job (or of
			// ConfirmExecutable, which already succeeded) with no caller left
			// to fail an install that just finished; Record already flipped
			// history into Degraded and emitted history:degraded, so the
			// user learns about the journal problem from the banner.
			slog.Error("record install history", "id", id, "error", err)
		}
	}
	return nil
}

func (s *Service) register(item Installation, version, source string) (library.Game, error) {
	if s.library == nil {
		return library.Game{}, errNoLibrary
	}
	if item.Destination == "" {
		return library.Game{}, errEmptyDestination
	}
	title := s.titleOf(item.Origin)
	if title == "" {
		title = item.Name
	}
	return s.library.RegisterInstalled(library.InstalledGame{
		Title:             title,
		Executable:        item.Executable,
		InstallDir:        item.Destination,
		Version:           version,
		VersionSource:     source,
		SourceDownloadID:  item.DownloadID,
		ReleaseID:         item.Origin.ReleaseID,
		SourceID:          item.Origin.SourceID,
		DistributionID:    item.Origin.DistributionID,
		ReleaseUploadedAt: item.Origin.ReleaseUploadedAt,
		CanonicalGameID:   item.Origin.GameID,
		Repacker:          s.repackerOf(item.Origin.ReleaseID),
		ReleaseVersion:    item.Origin.Version,
		InstallType:       string(item.Type),
		Owned:             item.Owned,
		Uninstall:         item.Uninstall,
		UninstallUnknown:  item.UninstallUnknown,
	})
}

func (s *Service) applyCleanup(cfg settings.Settings, downloadID string) {
	if cfg.InstallCleanupPolicy != settings.CleanupDelete || s.downloads == nil {
		return
	}
	d, err := s.downloads.Get(downloadID)
	if err != nil {
		slog.Error("cleanup lookup download", "id", downloadID, "error", err)
		return
	}
	if d.Seeding {
		slog.Info("cleanup skipped, download is seeding", "id", downloadID)
		return
	}
	if err := s.downloads.DeleteData(downloadID); err != nil {
		slog.Warn("cleanup download data", "id", downloadID, "error", err)
		return
	}
	slog.Info("download data removed after install", "id", downloadID)
}

// candidatePaths — то, что лаунчер предложил на выбор. Один только счётчик в
// журнале не отвечает на первый вопрос разбора «игра не запускается»: что
// именно было предложено и что из этого запускается.
func candidatePaths(candidates []Candidate) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, fmt.Sprintf("%s (%.0f)", c.Path, c.Score))
	}
	return out
}

func (s *Service) setExecutable(id, executable string, candidates []Candidate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil {
		return nil
	}
	prevExecutable, prevCandidates := item.Executable, item.Candidates
	item.Executable = executable
	item.Candidates = candidates
	if err := s.persistLocked(); err != nil {
		item.Executable = prevExecutable
		item.Candidates = prevCandidates
		return wrapPersistError(err)
	}
	slog.Info("install executable chosen", "id", id, "executable", executable, "candidates", len(candidates))
	return nil
}

func (s *Service) installerLogPath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return filepath.Join(s.store.dir, "installer-"+id+".log")
}

func (s *Service) workerStatePath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return workerStatePath(s.store.dir, id)
}

func (s *Service) workerInfPath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return workerInfPath(s.store.dir, id)
}

func (s *Service) workerCancelPath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return workerCancelPath(s.store.dir, id)
}

func (s *Service) workerSpecFilePath(id string) string {
	if s.store == nil || s.store.dir == "" {
		return ""
	}
	return workerSpecFilePath(s.store.dir, id)
}

// workerFiles — всё, что лаунчер и воркер оставляют на диске ради одного
// прогона. Пустые пути отбрасываются в removeWorkerFiles.
func (s *Service) workerFiles(id string) []string {
	return []string{s.workerSpecFilePath(id), s.workerStatePath(id), s.workerCancelPath(id), s.workerInfPath(id)}
}

// releaseWorkerFiles убирает файлы воркера, когда итог прогона уже в записи.
// Остаются они в двух случаях: воркер не подтвердил остановку и мог не
// закончить писать (по ним Retry и Cancel узнают, что он жив), либо запись ещё в
// работе и после перезапуска её продолжат по этим файлам. Статус читается под
// тем же замком, что и удаление: Retry не успеет занять запись между ними.
func (s *Service) releaseWorkerFiles(id string, cause error) {
	if errors.Is(cause, errInstallerNotConfirmedStopped) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || transient(item.Status) {
		return
	}
	if err := removeWorkerFiles(s.workerFiles(id)...); err != nil {
		slog.Warn("remove worker files of a settled install", "id", id, "error", err)
	}
}

func (s *Service) forceDestination(id, destination string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || destination == "" {
		return nil
	}
	prev := item.Destination
	item.Destination = destination
	if err := s.persistLocked(); err != nil {
		item.Destination = prev
		return wrapPersistError(err)
	}
	return nil
}

func (s *Service) setDestination(id, destination string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.findLocked(id)
	if item == nil || item.Destination != "" {
		return nil
	}
	item.Destination = destination
	if err := s.persistLocked(); err != nil {
		item.Destination = ""
		return wrapPersistError(err)
	}
	return nil
}

const (
	VersionSourceRelease    = "release_metadata"
	VersionSourceExecutable = "executable_metadata"
)

func verifyInstall(item Installation) error {
	if item.Destination != "" {
		entries, err := os.ReadDir(item.Destination)
		if err != nil {
			return fmt.Errorf("чтение папки установки: %w", err)
		}
		if len(entries) == 0 {
			return errEmptyInstall
		}
	}
	if item.Executable == "" {
		return nil
	}
	destination := ""
	if ownDestination(item.Type, item.Silent) {
		destination = item.Destination
	}
	return validExecutable(item.Executable, destination, item.Type)
}

func detectVersion(item Installation) (string, string) {
	if item.Origin.Version != "" {
		return item.Origin.Version, VersionSourceRelease
	}
	if item.Executable != "" {
		if info, ok := ExeVersion(item.Executable); ok && info.Version != "" {
			return info.Version, info.Source
		}
	}
	if item.Destination != "" {
		if info, ok := VersionFromFiles(item.Destination); ok {
			return info.Version, info.Source
		}
	}
	return "", ""
}

func gather(ctx context.Context, dirs []string, title string) ([]Candidate, error) {
	var out []Candidate
	for _, dir := range dirs {
		found, err := FindExecutables(ctx, dir, title)
		if err != nil {
			return nil, err
		}
		out = append(out, found...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Path < out[j].Path
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out, nil
}

func pickInstallDir(dirs []string, candidates []Candidate) string {
	if len(dirs) == 0 {
		return ""
	}
	if len(candidates) > 0 {
		for _, dir := range dirs {
			if inside(dir, candidates[0].Path) {
				return dir
			}
		}
		return ""
	}
	if len(dirs) == 1 {
		return dirs[0]
	}
	return ""
}

var removeInstalledSource = os.RemoveAll

// adoptJob заводит запись job для продолжения цепочки после перезапуска:
// без неё Cancel не видел бы идущий установщик и объявил бы отмену над
// процессом, который продолжает писать.
func (s *Service) adoptJob(ctx context.Context, id string) (context.Context, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs[id] != nil {
		return ctx, func() {}
	}
	jobCtx, cancel := context.WithCancel(ctx)
	s.jobs[id] = &job{cancel: cancel}
	return jobCtx, func() { s.endJob(id) }
}
