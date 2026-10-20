package library

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"typhon/internal/uierr"
)

const MaxNotesChars = 4000

var (
	ErrNotesTooLong = uierr.New("library.notes_too_long", "заметка слишком длинная")
	ErrNotesInvalid = uierr.New("library.notes_invalid", "заметка содержит недопустимые символы")
)

func (s *Service) NotesLimit() int {
	return MaxNotesChars
}

func normalizeNotes(text string) (string, error) {
	if !utf8.ValidString(text) {
		return "", ErrNotesInvalid
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimRightFunc(text, unicode.IsSpace)
	for _, r := range text {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return "", ErrNotesInvalid
		}
	}
	if utf8.RuneCountInString(text) > MaxNotesChars {
		return "", ErrNotesTooLong
	}
	return text, nil
}

func (s *Service) SetNotes(id, text string) error {
	notes, err := normalizeNotes(text)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	game := s.findLocked(id)
	if game == nil {
		return errNotFound
	}
	if game.Notes == notes {
		return nil
	}
	previous := *game
	game.Notes = notes
	if err := s.persist(); err != nil {
		*game = previous
		return fmt.Errorf("save library: %w", err)
	}
	s.emitUpdated()
	return nil
}
