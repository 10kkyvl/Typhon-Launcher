package metadata

import "context"

type languageKey struct{}

func requestLanguage(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, languageKey{}, lang)
}

// Language returns the UI language captured when this operation started.
func Language(ctx context.Context) string {
	if lang, _ := ctx.Value(languageKey{}).(string); lang == "ru" {
		return lang
	}
	return "en"
}

// SetLanguage receives the resolved launcher locale, including System mode.
func (s *Service) SetLanguage(lang string) {
	if lang != "ru" {
		lang = "en"
	}
	s.mu.Lock()
	s.language = lang
	s.mu.Unlock()
}
func (s *Service) currentLanguage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.language == "ru" {
		return "ru"
	}
	return "en"
}
