package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"typhon/internal/dialogtext"
	"typhon/internal/storage"
	"typhon/internal/uierr"
	"typhon/skills"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var ErrSkillPath = uierr.New("theme.skill_invalid_path", "недопустимая папка для скилла")

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
	root := filepath.Join(dir, skills.ThemeDir)
	err = fs.WalkDir(skills.FS, skills.ThemeDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		dst := filepath.Join(root, filepath.FromSlash(path.Clean(p[len(skills.ThemeDir):])))
		if d.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("создать каталог %s: %w", dst, err)
			}
			return nil
		}
		data, err := fs.ReadFile(skills.FS, p)
		if err != nil {
			return fmt.Errorf("прочитать %s из встроенного скилла: %w", p, err)
		}
		if err := storage.WriteAtomic(dst, data); err != nil {
			return fmt.Errorf("записать %s: %w", dst, err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return root, nil
}

func (s *Service) SelectSkillFolder() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errNoDialog
	}
	dialog := app.Dialog.OpenFile().
		SetTitle("Typhon: папка скиллов ИИ").
		CanChooseDirectories(true).
		CanChooseFiles(false)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("домашний каталог пользователя: %w", err)
	}
	def := filepath.Join(home, ".claude", "skills")
	switch _, err := os.Stat(def); {
	case err == nil:
		dialog.SetDirectory(def)
	case !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("проверить %s: %w", def, err)
	}
	picked, err := dialogtext.Selection(dialog.PromptForSingleSelection())
	if err != nil {
		return "", fmt.Errorf("выбор папки скилла: %w", err)
	}
	return picked, nil
}
