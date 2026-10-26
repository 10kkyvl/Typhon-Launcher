package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"typhon/internal/dialogtext"
	"typhon/internal/storage"
	"typhon/internal/uierr"
	"typhon/skills"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	ErrSkillPath  = uierr.New("theme.skill_invalid_path", "недопустимая папка для скилла")
	ErrSkillWrite = uierr.New("theme.skill_write_failed", "не удалось записать скилл в выбранную папку")
)

const skillStagePattern = ".typhon-theme-*.tmp"

func (s *Service) SaveAISkill(dir string) (string, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%q: %w", dir, ErrSkillPath)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSkillPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q не каталог: %w", dir, ErrSkillPath)
	}
	stage, err := os.MkdirTemp(dir, skillStagePattern)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSkillWrite, err)
	}
	defer func() {
		if err := os.RemoveAll(stage); err != nil {
			slog.Warn("remove skill staging folder", "path", stage, "error", err)
		}
	}()
	files, err := stageSkill(stage)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSkillWrite, err)
	}
	root := filepath.Join(dir, skills.ThemeDir)
	if err := installSkill(stage, root, files); err != nil {
		return "", fmt.Errorf("%w: %w", ErrSkillWrite, err)
	}
	return root, nil
}

func stageSkill(stage string) ([]string, error) {
	var files []string
	err := fs.WalkDir(skills.FS, skills.ThemeDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(path.Clean(p), skills.ThemeDir)
		rel = strings.TrimPrefix(rel, "/")
		dst := filepath.Join(stage, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := fs.ReadFile(skills.FS, p)
		if err != nil {
			return fmt.Errorf("прочитать %s из встроенного скилла: %w", p, err)
		}
		if err := storage.WriteAtomic(dst, data); err != nil {
			return err
		}
		files = append(files, filepath.FromSlash(rel))
		return nil
	})
	return files, err
}

// installSkill moves staged files into root one by one; a file already there
// is parked in backup first and put back if any later step fails, so a failed
// install leaves the previous skill as it was and never touches foreign files.
func installSkill(stage, root string, files []string) (err error) {
	backup := filepath.Join(stage, ".backup")
	type moved struct {
		dst, saved string
		hadOld     bool
	}
	var done []moved
	defer func() {
		if err == nil {
			return
		}
		for i := len(done) - 1; i >= 0; i-- {
			m := done[i]
			if rmErr := os.Remove(m.dst); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
				err = errors.Join(err, rmErr)
				continue
			}
			if m.hadOld {
				if rbErr := os.Rename(m.saved, m.dst); rbErr != nil {
					err = errors.Join(err, rbErr)
				}
			}
		}
	}()
	for _, rel := range files {
		dst := filepath.Join(root, rel)
		if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			return mkErr
		}
		m := moved{dst: dst, saved: filepath.Join(backup, rel)}
		if _, statErr := os.Lstat(dst); statErr == nil {
			if mkErr := os.MkdirAll(filepath.Dir(m.saved), 0o755); mkErr != nil {
				return mkErr
			}
			if mvErr := os.Rename(dst, m.saved); mvErr != nil {
				return mvErr
			}
			m.hadOld = true
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		done = append(done, m)
		if mvErr := os.Rename(filepath.Join(stage, rel), dst); mvErr != nil {
			return mvErr
		}
	}
	return nil
}

func (s *Service) SelectSkillFolder(language string) (string, error) {
	app := application.Get()
	if app == nil {
		return "", errNoDialog
	}
	title := dialogtext.For(language).SkillFolder
	dialog := app.Dialog.OpenFile().
		SetTitle(title).
		SetMessage(title).
		CanChooseDirectories(true).
		CanChooseFiles(false)
	// Only the dialog's starting folder depends on this; failing to find it
	// must not stop the user from choosing a folder by hand.
	if home, err := os.UserHomeDir(); err != nil {
		slog.Warn("skill folder dialog: home directory", "error", err)
	} else {
		def := filepath.Join(home, ".claude", "skills")
		switch _, err := os.Stat(def); {
		case err == nil:
			dialog.SetDirectory(def)
		case !errors.Is(err, fs.ErrNotExist):
			slog.Warn("skill folder dialog: default folder", "path", def, "error", err)
		}
	}
	picked, err := dialogtext.Selection(dialog.PromptForSingleSelection())
	if err != nil {
		return "", fmt.Errorf("выбор папки скилла: %w", err)
	}
	return picked, nil
}
