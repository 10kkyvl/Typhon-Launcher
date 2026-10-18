package library

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"typhon/internal/uierr"
)

const (
	MaxTags   = 20
	MaxTagLen = 32
)

var (
	ErrTooManyTags = uierr.New("library.too_many_tags", "tags limit reached")
	ErrTagTooLong  = uierr.New("library.tag_too_long", "tag is too long")
	ErrTagInvalid  = uierr.New("library.tag_invalid", "tag contains control characters")
)

func NormalizeTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		for _, r := range raw {
			if unicode.IsControl(r) && !unicode.IsSpace(r) {
				return nil, ErrTagInvalid
			}
		}
		tag := strings.Join(strings.Fields(raw), " ")
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > MaxTagLen {
			return nil, ErrTagTooLong
		}
		key := strings.ToLower(tag)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	if len(out) > MaxTags {
		return nil, ErrTooManyTags
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (s *Service) SetTags(id string, tags []string) (Game, error) {
	normalized, err := NormalizeTags(tags)
	if err != nil {
		return Game{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	previous := *game
	game.Tags = normalized
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func (s *Service) SetHidden(id string, on bool) (Game, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return Game{}, errNotFound
	}
	if game.Hidden == on {
		return *game, nil
	}
	previous := *game
	game.Hidden = on
	if err := s.persist(); err != nil {
		*game = previous
		return Game{}, fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return *game, nil
}

func (s *Service) AllTags() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]struct{}{}
	out := []string{}
	for _, g := range s.games {
		for _, tag := range g.Tags {
			key := strings.ToLower(tag)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, tag)
		}
	}
	return out
}
