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

	// Broker — уже поднятый повышенный процесс этой загрузки, если права
	// запрашивали заранее. Есть он или нет, дальше всё одинаково: лаунчер
	// кладёт задание и читает состояние из файлов, потому что процессом с
	// правами администратора он не владеет ни в том, ни в другом случае.
	Broker *brokerHandoff
}

type brokerHandoff struct {
	Dir  string
	Gone <-chan struct{}
}

type runner interface {
	run(ctx context.Context, spec runSpec) (int, error)
}
