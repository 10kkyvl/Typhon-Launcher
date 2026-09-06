package install

import (
	"context"
)

type runSpec struct {
	Path    string
	Args    []string
	Dir     string
	CmdLine string
	// Хвост командной строки без argv[0]: ShellExecuteEx принимает параметры
	// отдельно от файла, а собрать их из CmdLine нельзя — там ключи, которые
	// нельзя ни разбивать, ни экранировать.
	Tail       string
	Background bool
	Hidden     bool

	// Поля ниже нужны только повышенному воркеру (elevated.go, worker_run.go):
	// без прав администратора процесс с установщиком
	// лаунчеру не принадлежит, поэтому воркер получает не готовую команду, а
	// данные, из которых сам строит и разведку компонентов, и основной прогон.
	ID            string
	Engine        Engine
	InstallerPath string
	Destination   string
	LogPath       string
	Options       installOptions
	StatePath     string
	InfPath       string
	CancelPath    string
}

type runner interface {
	run(ctx context.Context, spec runSpec) (int, error)
}

// discovery сводит runSpec к discoverySpec (worker.go), чтобы разведкой
// компонентов Inno пользовались все пути запуска сразу: неэлевированный на
// Windows, повышенный воркер и запуск в бутыле на macOS (инвариант 28 — один
// источник правды на понятие).
func (s runSpec) discovery() discoverySpec {
	return discoverySpec{
		Engine: s.Engine, InstallerPath: s.InstallerPath, Destination: s.Destination,
		WorkingDir: s.Dir, InfPath: s.InfPath, Options: s.Options,
	}
}
