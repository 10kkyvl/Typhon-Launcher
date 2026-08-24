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
	ID    string
	Title string
	Body  string
}

var Required = []Meta{
	{ID: "terms", Title: "Условия использования"},
	{ID: "privacy", Title: "Политика конфиденциальности"},
	{ID: "copyright", Title: "Авторские права и обращения"},
	{ID: "third-party", Title: "Лицензии сторонних компонентов"},
}

var files = map[string]string{
	"terms":       "TERMS.md",
	"privacy":     "PRIVACY.md",
	"copyright":   "COPYRIGHT.md",
	"third-party": "THIRD_PARTY_NOTICES.md",
}

var ErrUnknownDocument = errors.New("legal: unknown document")

type Service struct {
	docs map[string]Document
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

func load(fsys fs.FS) (map[string]Document, error) {
	if fsys == nil {
		return nil, errors.New("legal: filesystem is nil")
	}
	docs := make(map[string]Document, len(Required))
	for _, meta := range Required {
		name, ok := files[meta.ID]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownDocument, meta.ID)
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("legal: read %s: %w", name, err)
		}
		body := string(data)
		if strings.TrimSpace(body) == "" {
			return nil, fmt.Errorf("legal: document %s is empty", meta.ID)
		}
		docs[meta.ID] = Document{ID: meta.ID, Title: meta.Title, Body: body}
	}
	return docs, nil
}

func (s *Service) List() []Meta {
	out := make([]Meta, len(Required))
	copy(out, Required)
	return out
}

func (s *Service) Get(id string) (Document, error) {
	doc, ok := s.docs[id]
	if !ok {
		return Document{}, fmt.Errorf("%w: %s", ErrUnknownDocument, id)
	}
	return doc, nil
}
