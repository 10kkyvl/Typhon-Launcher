package install

import (
	"context"
	"crypto/ed25519"

	"typhon/internal/installguard"
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
	// Outlive оставляет процесс жить после смерти лаунчера. Деинсталлятор,
	// убитый посередине, оставляет полуудалённую игру при прежней записи в
	// библиотеке; установщик, наоборот, гасится вместе с владельцем.
	Outlive bool

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

	// Shell передаёт воркеру уборку ярлыков в общих каталогах и забирает итог.
	// Без повышения он не используется: тогда уборка целиком остаётся за лаунчером.
	Shell *shellHandoff
}

type brokerHandoff struct {
	Key  ed25519.PrivateKey
	Dir  string
	Gone <-chan struct{}
}

type runner interface {
	run(ctx context.Context, spec runSpec) (int, error)
}

func bridgeFor(opts installOptions, hide, limit32 bool) installguard.Bridge {
	return installguard.Bridge{
		Options:                installguard.Options{HideProgress: hide, VerifyRepack: opts.VerifyRepack},
		Limit32BitAddressSpace: limit32,
	}
}

func bridgeArgs(bridge installguard.Bridge, cancelFile, installer string, args []string) []string {
	return append([]string{bridge.Mode(), cancelFile, "--", installer}, args...)
}
