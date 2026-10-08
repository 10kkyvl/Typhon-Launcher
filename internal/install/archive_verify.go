package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	rardecode "github.com/nwaples/rardecode/v2"

	"typhon/internal/platform"
)

const verifyRetries = 10

var (
	errArchiveHardLinked = errors.New("жёсткая ссылка на файл")
	errArchiveUnlisted   = errors.New("записи нет в оглавлении архива, вероятно, файловая система изменила имя")
	errFileGone          = errors.New("файл пропал после распаковки: его мог удалить антивирус или другая программа")
	errToolOutput        = errors.New("результат распаковки не совпадает с оглавлением архива")
)

var (
	syncExtracted     = syncAndCountLinks
	fileBusy          = osFileBusy
	verifyRetryDelay  = 500 * time.Millisecond
	verifyRetryBudget = 30 * time.Second
)

type listedEntry struct {
	name      string
	size      int64
	sizeKnown bool
	dir       bool
}

func relativeLabel(dest, path string) string {
	rel, err := filepath.Rel(dest, path)
	if err != nil {
		return entryLabel(filepath.ToSlash(path))
	}
	return entryLabel(filepath.ToSlash(rel))
}

func listedEntries(dest string, files []*rardecode.File) (map[string]listedEntry, error) {
	listed := make(map[string]listedEntry, len(files))
	for _, f := range files {
		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return nil, fmt.Errorf("%w: %w: %s", errArchiveUnsafe, err, entryLabel(f.Name))
		}
		key, err := platform.PathKey(target)
		if err != nil {
			return nil, verifyFailure(entryLabel(f.Name), err)
		}
		listed[key] = listedEntry{name: f.Name, size: f.UnPackedSize, sizeKnown: !f.UnKnownSize, dir: f.IsDir}
		for parent := filepath.Dir(target); inside(dest, parent) && !samePath(dest, parent); parent = filepath.Dir(parent) {
			parentKey, err := platform.PathKey(parent)
			if err != nil {
				return nil, verifyFailure(entryLabel(f.Name), err)
			}
			if _, ok := listed[parentKey]; !ok {
				listed[parentKey] = listedEntry{name: f.Name, dir: true}
			}
		}
	}
	return listed, nil
}

// Сбой самой проверки (диск, антивирус, обход каталога) — не вина распаковщика:
// другой распаковщик упрётся в то же самое, поэтому перебор на нём
// останавливается.
func verifyFailure(what string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %w: %s", errArchiveMismatch, errFileGone, what)
	}
	return fmt.Errorf("%w: %s: %w", errArchiveVerify, what, err)
}

// Распаковщик пишет мимо safeJoin, а rardecode не разбирает записи
// перенаправления RAR5, поэтому ссылки, которых нет в оглавлении, видны только
// здесь. Каждый файл заодно сбрасывается на диск, как это делает writeEntry у
// встроенного распаковщика.
func verifyExtracted(ctx context.Context, dest string, files []*rardecode.File, rep *reporter) error {
	listed, err := listedEntries(dest, files)
	if err != nil {
		return err
	}
	v := &verification{ctx: ctx, dest: dest, listed: listed, sizes: make(map[string]int64, len(files)), rep: rep, budget: verifyRetryBudget}
	if err := filepath.WalkDir(dest, v.walk); err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir {
			continue
		}
		target, err := safeJoin(dest, f.Name)
		if err != nil {
			return fmt.Errorf("%w: %w: %s", errArchiveUnsafe, err, entryLabel(f.Name))
		}
		key, err := platform.PathKey(target)
		if err != nil {
			return verifyFailure(entryLabel(f.Name), err)
		}
		got, ok := v.sizes[key]
		if !ok {
			return fmt.Errorf("%w: %w: %s", errArchiveMismatch, errFileGone, entryLabel(f.Name))
		}
		if want := listed[key]; want.sizeKnown && got != want.size {
			return fmt.Errorf("%w: %s: ожидалось %d байт, на диске %d", errToolOutput, entryLabel(f.Name), want.size, got)
		}
	}
	return nil
}

type verification struct {
	ctx    context.Context
	dest   string
	listed map[string]listedEntry
	sizes  map[string]int64
	rep    *reporter
	budget time.Duration
}

func (v *verification) walk(path string, d fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return verifyFailure("обход результата распаковки", walkErr)
	}
	if err := v.ctx.Err(); err != nil {
		return err
	}
	if path == v.dest {
		return nil
	}
	return v.entry(path, d)
}

func (v *verification) entry(path string, d fs.DirEntry) error {
	label := relativeLabel(v.dest, path)
	info, err := d.Info()
	if err != nil {
		return verifyFailure(label, err)
	}
	reparse, err := reparsePoint(info)
	if err != nil {
		return verifyFailure(label, err)
	}
	if reparse || (!info.Mode().IsRegular() && !info.IsDir()) {
		return fmt.Errorf("%w: %w: %s", errArchiveUnsafe, errArchiveHasLinks, label)
	}
	key, err := platform.PathKey(path)
	if err != nil {
		return verifyFailure(label, err)
	}
	entry, ok := v.listed[key]
	switch {
	case !ok:
		return fmt.Errorf("%w: %w: %s", errArchiveMismatch, errArchiveUnlisted, label)
	case info.IsDir() && !entry.dir:
		return fmt.Errorf("%w: %s: на месте файла каталог", errToolOutput, label)
	case info.IsDir():
		return nil
	case entry.dir:
		return fmt.Errorf("%w: %s: на месте каталога файл", errToolOutput, label)
	}
	v.rep.setFile(entry.name)
	links, err := v.sync(path, info.Mode())
	if err != nil {
		return verifyFailure(label, err)
	}
	if links > 1 {
		return fmt.Errorf("%w: %w: %s", errArchiveUnsafe, errArchiveHardLinked, label)
	}
	v.sizes[key] = info.Size()
	return nil
}

// Антивирус и индексатор держат только что записанный файл открытым, и сброс на
// диск упирается в нарушение совместного доступа: это проходит само. Ожиданий
// не больше verifyRetries на файл и verifyRetryBudget на всю распаковку, иначе
// сотни занятых файлов растянули бы её на часы; пауза прерывается отменой.
func (v *verification) sync(path string, mode fs.FileMode) (uint64, error) {
	for attempt := 0; ; attempt++ {
		links, err := syncExtracted(path, mode)
		if err == nil {
			return links, nil
		}
		if attempt >= verifyRetries || !fileBusy(err) || v.budget < verifyRetryDelay {
			return 0, err
		}
		v.budget -= verifyRetryDelay
		timer := time.NewTimer(verifyRetryDelay)
		select {
		case <-v.ctx.Done():
			timer.Stop()
			return 0, v.ctx.Err()
		case <-timer.C:
		}
	}
}
