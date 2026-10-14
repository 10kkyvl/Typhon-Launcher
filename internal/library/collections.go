package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"typhon/internal/storage"
	"typhon/internal/uierr"
)

const (
	collectionsName    = "collections.json"
	EventCollections   = "library:collections"
	MaxCollections     = 50
	MaxCollectionName  = 48
	MaxCollectionGenre = 20
)

var (
	ErrCollectionName     = uierr.New("library.collection_name_invalid", "collection name is empty or too long")
	ErrTooManyCollections = uierr.New("library.too_many_collections", "collections limit reached")
	ErrCollectionGenres   = uierr.New("library.collection_genres_invalid", "too many genres in a collection")
	ErrCollectionNotFound = uierr.New("library.collection_not_found", "collection not found")
)

type Collection struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Tags      []string  `json:"tags,omitempty"`
	Genres    []string  `json:"genres,omitempty"`
	Status    string    `json:"status,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func collectionsPathFor(libraryPath string) string {
	return filepath.Join(filepath.Dir(libraryPath), collectionsName)
}

func loadCollections(path string) ([]Collection, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read collections %s: %w", path, err)
	}
	var list []Collection
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse collections %s: %w", path, err)
	}
	return list, nil
}

func (s *Service) persistCollectionsLocked() error {
	if s.collectionsPath == "" {
		return errors.New("collections path unavailable")
	}
	list := s.collections
	if list == nil {
		list = []Collection{}
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := storage.WriteAtomic(s.collectionsPath, data); err != nil {
		return fmt.Errorf("write collections %s: %w", s.collectionsPath, err)
	}
	return nil
}

func cloneCollection(c Collection) Collection {
	c.Tags = append([]string(nil), c.Tags...)
	c.Genres = append([]string(nil), c.Genres...)
	return c
}

func (s *Service) collectionsCopyLocked() []Collection {
	out := make([]Collection, len(s.collections))
	for i, c := range s.collections {
		out[i] = cloneCollection(c)
	}
	return out
}

func (s *Service) GetCollections() []Collection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.collectionsCopyLocked()
}

func normalizeCollection(c Collection) (Collection, error) {
	c.Name = strings.Join(strings.Fields(c.Name), " ")
	if c.Name == "" || utf8.RuneCountInString(c.Name) > MaxCollectionName {
		return Collection{}, ErrCollectionName
	}
	tags, err := NormalizeTags(c.Tags)
	if err != nil {
		return Collection{}, err
	}
	c.Tags = tags
	if !ValidStatus(c.Status) {
		return Collection{}, ErrInvalidStatus
	}
	var genres []string
	seen := map[string]struct{}{}
	for _, g := range c.Genres {
		g = strings.Join(strings.Fields(g), " ")
		if g == "" {
			continue
		}
		if utf8.RuneCountInString(g) > MaxCollectionName || strings.ContainsFunc(g, unicode.IsControl) {
			return Collection{}, ErrCollectionGenres
		}
		key := strings.ToLower(g)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		genres = append(genres, g)
	}
	if len(genres) > MaxCollectionGenre {
		return Collection{}, ErrCollectionGenres
	}
	c.Genres = genres
	return c, nil
}

func (s *Service) SaveCollection(c Collection) (Collection, error) {
	c, err := normalizeCollection(c)
	if err != nil {
		return Collection{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	previous := s.collections
	next := s.collectionsCopyLocked()
	c.UpdatedAt = now
	if c.ID == "" {
		if len(next) >= MaxCollections {
			return Collection{}, ErrTooManyCollections
		}
		c.ID = newID()
		c.CreatedAt = now
		next = append(next, c)
	} else {
		idx := -1
		for i := range next {
			if next[i].ID == c.ID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return Collection{}, ErrCollectionNotFound
		}
		c.CreatedAt = next[idx].CreatedAt
		next[idx] = c
	}
	s.collections = next
	if err := s.persistCollectionsLocked(); err != nil {
		s.collections = previous
		return Collection{}, err
	}
	emit(EventCollections, s.collectionsCopyLocked())
	return cloneCollection(c), nil
}

func (s *Service) DeleteCollection(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i := range s.collections {
		if s.collections[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrCollectionNotFound
	}
	previous := s.collections
	next := s.collectionsCopyLocked()
	s.collections = append(next[:idx], next[idx+1:]...)
	if err := s.persistCollectionsLocked(); err != nil {
		s.collections = previous
		return err
	}
	emit(EventCollections, s.collectionsCopyLocked())
	return nil
}
