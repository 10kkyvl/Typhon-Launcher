package install

import (
	"context"
	"errors"
)

var (
	errElevationDeclined = errors.New("нужны права администратора: запрос Windows отклонён. Повторите действие и подтвердите запрос")
	errNoElevatedProcess = errors.New("процесс установщика с правами администратора не запустился")
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
}

type runner interface {
	run(ctx context.Context, spec runSpec) (int, error)
}
