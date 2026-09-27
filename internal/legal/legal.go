package legal

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

type Meta struct {
	ID    string
	Title string
}

type Document struct {
	ID     string
	Locale string
	Title  string
	Body   string
}

const (
	LocaleRU = "ru"
	LocaleEN = "en"
)

var Locales = []string{LocaleRU, LocaleEN}

// Title — запасной заголовок на языке документов: в UI он локализуется по ID
// через frontend/src/lib/services/legalMessages.ts.
var Required = []Meta{
	{ID: "terms", Title: "Условия использования"},
	{ID: "privacy", Title: "Политика конфиденциальности"},
	{ID: "copyright", Title: "Авторские права и обращения"},
	{ID: "third-party", Title: "Лицензии сторонних компонентов"},
}

var files = map[string]map[string]string{
	LocaleRU: {
		"terms":       "TERMS.md",
		"privacy":     "PRIVACY.md",
		"copyright":   "COPYRIGHT.md",
		"third-party": "THIRD_PARTY_NOTICES.md",
	},
	LocaleEN: {
		"terms":       "TERMS.en.md",
		"privacy":     "PRIVACY.en.md",
		"copyright":   "COPYRIGHT.en.md",
		"third-party": "THIRD_PARTY_NOTICES.en.md",
	},
}

var (
	ErrUnknownDocument = errors.New("legal: unknown document")
	ErrUnknownLocale   = errors.New("legal: unknown locale")
)

type Service struct {
	docs map[string]map[string]Document
}

func NewService(fsys fs.FS) (*Service, error) {
	docs, err := load(fsys)
	if err != nil {
		return nil, err
	}
	return &Service{docs: docs}, nil
}

func Validate(fsys fs.FS) error {
	_, err := load(fsys)
	return err
}

func load(fsys fs.FS) (map[string]map[string]Document, error) {
	if fsys == nil {
		return nil, errors.New("legal: filesystem is nil")
	}
	docs := make(map[string]map[string]Document, len(Locales))
	for _, locale := range Locales {
		names, ok := files[locale]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownLocale, locale)
		}
		byID := make(map[string]Document, len(Required))
		for _, meta := range Required {
			name, ok := names[meta.ID]
			if !ok {
				return nil, fmt.Errorf("%w: %s (%s)", ErrUnknownDocument, meta.ID, locale)
			}
			data, err := fs.ReadFile(fsys, name)
			if err != nil {
				return nil, fmt.Errorf("legal: read %s: %w", name, err)
			}
			body := string(data)
			if strings.TrimSpace(body) == "" {
				return nil, fmt.Errorf("legal: document %s is empty", name)
			}
			byID[meta.ID] = Document{ID: meta.ID, Locale: locale, Title: meta.Title, Body: body}
		}
		docs[locale] = byID
	}
	return docs, nil
}

func (s *Service) List() []Meta {
	out := make([]Meta, len(Required))
	copy(out, Required)
	return out
}

func (s *Service) Get(id string, locale string) (Document, error) {
	byID, ok := s.docs[locale]
	if !ok {
		return Document{}, fmt.Errorf("%w: %s", ErrUnknownLocale, locale)
	}
	doc, ok := byID[id]
	if !ok {
		return Document{}, fmt.Errorf("%w: %s", ErrUnknownDocument, id)
	}
	return doc, nil
}
