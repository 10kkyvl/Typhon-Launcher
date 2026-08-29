package theme

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"typhon/internal/settings"
	"typhon/internal/storage"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const stateVersion = 1

type state struct {
	Themes    []Theme `json:"themes"`
	Pending   string  `json:"pending"`
	Confirmed string  `json:"confirmed"`
}

type Service struct {
	mu        sync.Mutex
	path      string
	themes    []Theme
	pending   string
	confirmed string
}

func NewService() (*Service, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	if dir == "" {
		return nil, fmt.Errorf("%w: пустой каталог конфигурации", ErrThemePath)
	}
	return NewServiceAt(filepath.Join(dir, "themes.json"))
}

//wails:ignore
func NewServiceAt(path string) (*Service, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: путь к файлу тем не задан", ErrThemePath)
	}
	st, err := loadState(path)
	if err != nil {
		return nil, err
	}
	s := &Service{
		path:      path,
		themes:    st.Themes,
		pending:   st.Pending,
		confirmed: st.Confirmed,
	}
	if s.pending != "" && s.pending != s.confirmed {
		s.pending = ""
		if err := s.persist(); err != nil {
			return nil, fmt.Errorf("revert pending theme: %w", err)
		}
		emit("theme:reverted", defaultTheme())
	}
	return s, nil
}

func loadState(path string) (state, error) {
	var st state
	err := storage.Load(path, stateVersion, nil, &st)
	if errors.Is(err, fs.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, fmt.Errorf("load themes: %w", err)
	}
	return st, nil
}

func (s *Service) persist() error {
	st := state{Themes: s.themes, Pending: s.pending, Confirmed: s.confirmed}
	if err := storage.Save(s.path, stateVersion, st); err != nil {
		return fmt.Errorf("save themes: %w", err)
	}
	return nil
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

func (s *Service) listLocked() []Theme {
	all := make([]Theme, 0, len(presets)+len(s.themes))
	all = append(all, presets...)
	all = append(all, s.themes...)
	return all
}

func (s *Service) List() []Theme {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

func (s *Service) getLocked(id string) (Theme, error) {
	if t, ok := presetByID(id); ok {
		return t, nil
	}
	for _, t := range s.themes {
		if t.ID == id {
			return t, nil
		}
	}
	return Theme{}, fmt.Errorf("%q: %w", id, ErrThemeNotFound)
}

func (s *Service) Get(id string) (Theme, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}

func (s *Service) idTakenLocked(id string) bool {
	if _, ok := presetByID(id); ok {
		return true
	}
	for _, t := range s.themes {
		if t.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) uniqueIDLocked(base string) string {
	if !s.idTakenLocked(base) {
		return base
	}
	trimmed := base
	if len(trimmed) > 58 {
		trimmed = trimmed[:58]
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", trimmed, n)
		if !s.idTakenLocked(candidate) {
			return candidate
		}
	}
}

// upsertLocked inserts or replaces a user theme. If id collides with a
// built-in preset, editing that preset forks a copy under a new id instead
// of shadowing it, per the "presets cannot be overwritten" rule.
func (s *Service) upsertLocked(t Theme) (Theme, error) {
	id := t.ID
	if _, ok := presetByID(id); ok {
		id = s.uniqueIDLocked(id)
	}
	t.ID = id
	t.BuiltIn = false
	t.UpdatedAt = time.Now().UTC()

	before := s.themes
	themes := make([]Theme, len(s.themes))
	copy(themes, s.themes)
	replaced := false
	for i := range themes {
		if themes[i].ID == id {
			themes[i] = t
			replaced = true
			break
		}
	}
	if !replaced {
		themes = append(themes, t)
	}
	s.themes = themes
	if err := s.persist(); err != nil {
		s.themes = before
		return Theme{}, fmt.Errorf("сохранить тему %s: %w", id, err)
	}
	emit("theme:updated", t)
	emit("theme:list", s.listLocked())
	return t, nil
}

func (s *Service) Save(t Theme) (Theme, error) {
	if t.BuiltIn {
		return Theme{}, ErrThemeBuiltIn
	}
	normalized, err := Validate(t)
	if err != nil {
		return Theme{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertLocked(normalized)
}

func (s *Service) Delete(id string) error {
	if _, ok := presetByID(id); ok {
		return fmt.Errorf("%q: %w", id, ErrThemeBuiltIn)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, t := range s.themes {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("%q: %w", id, ErrThemeNotFound)
	}
	beforeThemes := s.themes
	beforePending, beforeConfirmed := s.pending, s.confirmed

	themes := make([]Theme, 0, len(s.themes)-1)
	themes = append(themes, s.themes[:idx]...)
	themes = append(themes, s.themes[idx+1:]...)
	s.themes = themes
	if s.pending == id {
		s.pending = ""
	}
	if s.confirmed == id {
		s.confirmed = ""
	}
	if err := s.persist(); err != nil {
		s.themes = beforeThemes
		s.pending, s.confirmed = beforePending, beforeConfirmed
		return fmt.Errorf("удалить тему %s: %w", id, err)
	}
	emit("theme:list", s.listLocked())
	return nil
}

func (s *Service) Apply(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getLocked(id); err != nil {
		return err
	}
	before := s.pending
	s.pending = id
	if err := s.persist(); err != nil {
		s.pending = before
		return fmt.Errorf("применить тему %s: %w", id, err)
	}
	return nil
}

func (s *Service) Confirm(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getLocked(id); err != nil {
		return err
	}
	beforePending, beforeConfirmed := s.pending, s.confirmed
	s.pending = ""
	s.confirmed = id
	if err := s.persist(); err != nil {
		s.pending, s.confirmed = beforePending, beforeConfirmed
		return fmt.Errorf("подтвердить тему %s: %w", id, err)
	}
	return nil
}

// Active returns the theme the frontend should currently apply: the pending
// (unconfirmed) theme if one is being tried, otherwise the last confirmed
// one, otherwise the built-in default. A pending or confirmed id that no
// longer resolves to any theme (deleted after being selected) also falls
// back to the default instead of failing the caller, mirroring the startup
// revert in NewServiceAt.
func (s *Service) Active() (Theme, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.pending
	if id == "" {
		id = s.confirmed
	}
	if id == "" {
		return defaultTheme(), nil
	}
	t, err := s.getLocked(id)
	if err != nil {
		if errors.Is(err, ErrThemeNotFound) {
			return defaultTheme(), nil
		}
		return Theme{}, err
	}
	return t, nil
}

func (s *Service) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	beforeThemes := s.themes
	beforePending, beforeConfirmed := s.pending, s.confirmed
	s.themes = nil
	s.pending = ""
	s.confirmed = ""
	if err := s.persist(); err != nil {
		s.themes = beforeThemes
		s.pending, s.confirmed = beforePending, beforeConfirmed
		return fmt.Errorf("сбросить темы: %w", err)
	}
	emit("theme:list", s.listLocked())
	return nil
}

func validateImportPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: пустой путь", ErrThemePath)
	}
	if !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("%w: относительный путь", ErrThemePath)
	}
	clean := filepath.Clean(trimmed)
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".typhontheme", ".json":
	default:
		return "", fmt.Errorf("%w: неподдерживаемое расширение файла", ErrThemePath)
	}
	return clean, nil
}

func validateExportPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: пустой путь", ErrThemePath)
	}
	if !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("%w: относительный путь", ErrThemePath)
	}
	return filepath.Clean(trimmed), nil
}

func (s *Service) Import(path string) (Theme, error) {
	abs, err := validateImportPath(path)
	if err != nil {
		return Theme{}, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return Theme{}, fmt.Errorf("открыть файл темы: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(f, MaxThemeBytes+1))
	closeErr := f.Close()
	if readErr != nil {
		return Theme{}, fmt.Errorf("прочитать файл темы: %w", readErr)
	}
	if closeErr != nil {
		return Theme{}, fmt.Errorf("закрыть файл темы: %w", closeErr)
	}
	if int64(len(data)) > MaxThemeBytes {
		return Theme{}, ErrThemeSize
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var payload file
	if err := dec.Decode(&payload); err != nil {
		return Theme{}, fmt.Errorf("разобрать файл темы: %w", err)
	}
	if payload.Version != 1 {
		return Theme{}, fmt.Errorf("%d: %w", payload.Version, ErrThemeVersion)
	}

	normalized, err := Validate(payload.Theme)
	if err != nil {
		return Theme{}, err
	}
	normalized.BuiltIn = false

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertLocked(normalized)
}

func (s *Service) Export(id, path string) error {
	dst, err := validateExportPath(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	t, err := s.getLocked(id)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	payload := file{Version: 1, Theme: t}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("сформировать файл темы: %w", err)
	}
	if err := storage.WriteAtomic(dst, data); err != nil {
		return fmt.Errorf("записать файл темы %s: %w", dst, err)
	}
	return nil
}

var errNoDialog = errors.New("диалог выбора файла недоступен")

func (s *Service) SelectThemeFile() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errNoDialog
	}
	path, err := app.Dialog.OpenFile().
		SetTitle("Выберите файл темы").
		CanChooseFiles(true).
		AddFilter("Файл темы (*.typhontheme, *.json)", "*.typhontheme;*.json").
		AddFilter("Все файлы", "*.*").
		PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("выбор файла темы: %w", err)
	}
	return path, nil
}

func (s *Service) SelectExportPath() (string, error) {
	app := application.Get()
	if app == nil {
		return "", errNoDialog
	}
	path, err := app.Dialog.SaveFile().
		SetMessage("Сохранить тему").
		SetFilename("theme.typhontheme").
		AddFilter("Файл темы (*.typhontheme)", "*.typhontheme").
		PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("выбор пути экспорта темы: %w", err)
	}
	return path, nil
}
