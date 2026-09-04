package legal

import (
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"unicode"
)

func validFS() fstest.MapFS {
	return fstest.MapFS{
		"TERMS.md":                  {Data: []byte("# Условия использования")},
		"PRIVACY.md":                {Data: []byte("# Политика конфиденциальности")},
		"COPYRIGHT.md":              {Data: []byte("# Авторские права")},
		"THIRD_PARTY_NOTICES.md":    {Data: []byte("# Лицензии сторонних компонентов")},
		"TERMS.en.md":               {Data: []byte("# Terms of Use")},
		"PRIVACY.en.md":             {Data: []byte("# Privacy Policy")},
		"COPYRIGHT.en.md":           {Data: []byte("# Copyright Notices")},
		"THIRD_PARTY_NOTICES.en.md": {Data: []byte("# Third-Party Components")},
	}
}

func TestNewService(t *testing.T) {
	cases := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr bool
	}{
		{
			name:    "all present",
			fsys:    validFS(),
			wantErr: false,
		},
		{
			name: "missing terms",
			fsys: func() fstest.MapFS {
				m := validFS()
				delete(m, "TERMS.md")
				return m
			}(),
			wantErr: true,
		},
		{
			name: "missing privacy",
			fsys: func() fstest.MapFS {
				m := validFS()
				delete(m, "PRIVACY.md")
				return m
			}(),
			wantErr: true,
		},
		{
			name: "missing copyright",
			fsys: func() fstest.MapFS {
				m := validFS()
				delete(m, "COPYRIGHT.md")
				return m
			}(),
			wantErr: true,
		},
		{
			name: "missing third party",
			fsys: func() fstest.MapFS {
				m := validFS()
				delete(m, "THIRD_PARTY_NOTICES.md")
				return m
			}(),
			wantErr: true,
		},
		{
			name: "missing english terms",
			fsys: func() fstest.MapFS {
				m := validFS()
				delete(m, "TERMS.en.md")
				return m
			}(),
			wantErr: true,
		},
		{
			name: "missing english third party",
			fsys: func() fstest.MapFS {
				m := validFS()
				delete(m, "THIRD_PARTY_NOTICES.en.md")
				return m
			}(),
			wantErr: true,
		},
		{
			name: "empty english privacy",
			fsys: func() fstest.MapFS {
				m := validFS()
				m["PRIVACY.en.md"] = &fstest.MapFile{Data: []byte("  \n ")}
				return m
			}(),
			wantErr: true,
		},
		{
			name: "empty copyright",
			fsys: func() fstest.MapFS {
				m := validFS()
				m["COPYRIGHT.md"] = &fstest.MapFile{Data: []byte("")}
				return m
			}(),
			wantErr: true,
		},
		{
			name: "whitespace only privacy",
			fsys: func() fstest.MapFS {
				m := validFS()
				m["PRIVACY.md"] = &fstest.MapFile{Data: []byte("   \n\t  ")}
				return m
			}(),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewService(tc.fsys)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewService(%s): expected error, got nil", tc.name)
				}
				if svc != nil {
					t.Fatalf("NewService(%s): expected nil service on error, got %+v", tc.name, svc)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewService(%s): unexpected error: %v", tc.name, err)
			}
			if svc == nil {
				t.Fatalf("NewService(%s): expected non-nil service", tc.name)
			}
		})
	}
}

func TestNewService_NilFS(t *testing.T) {
	svc, err := NewService(nil)
	if err == nil {
		t.Fatal("NewService(nil): expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("NewService(nil): expected nil service, got %+v", svc)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(validFS()); err != nil {
		t.Fatalf("Validate(valid): unexpected error: %v", err)
	}

	m := validFS()
	delete(m, "TERMS.md")
	if err := Validate(m); err == nil {
		t.Fatal("Validate(missing terms): expected error, got nil")
	}
}

func TestService_ListOrder(t *testing.T) {
	svc, err := NewService(validFS())
	if err != nil {
		t.Fatalf("NewService: unexpected error: %v", err)
	}

	want := []string{"terms", "privacy", "copyright", "third-party"}
	got := svc.List()
	if len(got) != len(want) {
		t.Fatalf("List(): got %d entries, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("List()[%d].ID = %q, want %q", i, got[i].ID, id)
		}
		if got[i].Title == "" {
			t.Fatalf("List()[%d].Title is empty for id %q", i, id)
		}
	}
}

func TestService_Get(t *testing.T) {
	svc, err := NewService(validFS())
	if err != nil {
		t.Fatalf("NewService: unexpected error: %v", err)
	}

	for _, meta := range Required {
		t.Run(meta.ID, func(t *testing.T) {
			doc, err := svc.Get(meta.ID, LocaleRU)
			if err != nil {
				t.Fatalf("Get(%q): unexpected error: %v", meta.ID, err)
			}
			if doc.ID != meta.ID {
				t.Fatalf("Get(%q).ID = %q", meta.ID, doc.ID)
			}
			if doc.Title != meta.Title {
				t.Fatalf("Get(%q).Title = %q, want %q", meta.ID, doc.Title, meta.Title)
			}
			if strings.TrimSpace(doc.Body) == "" {
				t.Fatalf("Get(%q).Body is empty", meta.ID)
			}
		})
	}
}

func TestService_GetUnknown(t *testing.T) {
	svc, err := NewService(validFS())
	if err != nil {
		t.Fatalf("NewService: unexpected error: %v", err)
	}

	cases := []string{"", "unknown", "TERMS", "terms.md", " terms"}
	for _, id := range cases {
		t.Run("id="+id, func(t *testing.T) {
			doc, err := svc.Get(id, LocaleRU)
			if err == nil {
				t.Fatalf("Get(%q): expected error, got nil (doc=%+v)", id, doc)
			}
			if !errors.Is(err, ErrUnknownDocument) {
				t.Fatalf("Get(%q): error = %v, want wrapping ErrUnknownDocument", id, err)
			}
		})
	}
}

func TestService_GetLocales(t *testing.T) {
	svc, err := NewService(validFS())
	if err != nil {
		t.Fatalf("NewService: unexpected error: %v", err)
	}

	for _, meta := range Required {
		t.Run(meta.ID, func(t *testing.T) {
			ru, err := svc.Get(meta.ID, LocaleRU)
			if err != nil {
				t.Fatalf("Get(%q, ru): unexpected error: %v", meta.ID, err)
			}
			en, err := svc.Get(meta.ID, LocaleEN)
			if err != nil {
				t.Fatalf("Get(%q, en): unexpected error: %v", meta.ID, err)
			}
			if en.Locale != LocaleEN || ru.Locale != LocaleRU {
				t.Fatalf("Get(%q): locales = %q and %q", meta.ID, ru.Locale, en.Locale)
			}
			if en.Body == ru.Body {
				t.Fatalf("Get(%q): english body equals the russian one", meta.ID)
			}
		})
	}
}

func TestService_GetUnknownLocale(t *testing.T) {
	svc, err := NewService(validFS())
	if err != nil {
		t.Fatalf("NewService: unexpected error: %v", err)
	}

	for _, locale := range []string{"", "de", "RU", "en-US", " en"} {
		t.Run("locale="+locale, func(t *testing.T) {
			doc, err := svc.Get("terms", locale)
			if err == nil {
				t.Fatalf("Get(terms, %q): expected error, got nil (doc=%+v)", locale, doc)
			}
			if !errors.Is(err, ErrUnknownLocale) {
				t.Fatalf("Get(terms, %q): error = %v, want wrapping ErrUnknownLocale", locale, err)
			}
		})
	}
}

var devAddressMarkers = []string{
	"localhost",
	"127.0.0.1",
	"0.0.0.0",
	"::1",
	":8080",
}

func TestRealDocuments_NoDevAddresses(t *testing.T) {
	svc, err := NewService(os.DirFS("../.."))
	if err != nil {
		t.Fatalf("NewService(real docs): unexpected error: %v", err)
	}

	for _, locale := range Locales {
		for _, meta := range Required {
			doc, err := svc.Get(meta.ID, locale)
			if err != nil {
				t.Fatalf("Get(%q, %q): unexpected error: %v", meta.ID, locale, err)
			}
			lower := strings.ToLower(doc.Body)
			for _, marker := range devAddressMarkers {
				if strings.Contains(lower, strings.ToLower(marker)) {
					t.Errorf("document %q (%s) contains dev address marker %q", meta.ID, locale, marker)
				}
			}
		}
	}
}

func TestRealDocuments_AllPresentAndNonEmpty(t *testing.T) {
	svc, err := NewService(os.DirFS("../.."))
	if err != nil {
		t.Fatalf("NewService(real docs): unexpected error: %v", err)
	}

	got := svc.List()
	if len(got) != len(Required) {
		t.Fatalf("List(): got %d entries, want %d", len(got), len(Required))
	}

	for _, locale := range Locales {
		for _, meta := range Required {
			doc, err := svc.Get(meta.ID, locale)
			if err != nil {
				t.Fatalf("Get(%q, %q): unexpected error: %v", meta.ID, locale, err)
			}
			if strings.TrimSpace(doc.Body) == "" {
				t.Fatalf("real document %q (%s) is empty", meta.ID, locale)
			}
		}
	}
}

func TestRealDocuments_EnglishHasNoCyrillic(t *testing.T) {
	svc, err := NewService(os.DirFS("../.."))
	if err != nil {
		t.Fatalf("NewService(real docs): unexpected error: %v", err)
	}

	for _, meta := range Required {
		doc, err := svc.Get(meta.ID, LocaleEN)
		if err != nil {
			t.Fatalf("Get(%q, en): unexpected error: %v", meta.ID, err)
		}
		for _, r := range doc.Body {
			if unicode.Is(unicode.Cyrillic, r) {
				t.Errorf("english document %q contains cyrillic %q", meta.ID, string(r))
				break
			}
		}
	}
}

func TestRealDocuments_LocalesDifferAndShareStructure(t *testing.T) {
	svc, err := NewService(os.DirFS("../.."))
	if err != nil {
		t.Fatalf("NewService(real docs): unexpected error: %v", err)
	}

	for _, meta := range Required {
		ru, err := svc.Get(meta.ID, LocaleRU)
		if err != nil {
			t.Fatalf("Get(%q, ru): unexpected error: %v", meta.ID, err)
		}
		en, err := svc.Get(meta.ID, LocaleEN)
		if err != nil {
			t.Fatalf("Get(%q, en): unexpected error: %v", meta.ID, err)
		}
		if ru.Body == en.Body {
			t.Errorf("document %q: english body equals the russian one", meta.ID)
		}
		if got, want := headings(en.Body), headings(ru.Body); got != want {
			t.Errorf("document %q: english has %d headings, russian %d", meta.ID, got, want)
		}
	}
}

func TestRealDocuments_RevisionDatesMatchAcrossLocales(t *testing.T) {
	svc, err := NewService(os.DirFS("../.."))
	if err != nil {
		t.Fatalf("NewService(real docs): unexpected error: %v", err)
	}

	for _, id := range []string{"terms", "privacy", "copyright"} {
		ru, err := svc.Get(id, LocaleRU)
		if err != nil {
			t.Fatalf("Get(%q, ru): unexpected error: %v", id, err)
		}
		en, err := svc.Get(id, LocaleEN)
		if err != nil {
			t.Fatalf("Get(%q, en): unexpected error: %v", id, err)
		}
		ruDate := revisionNumbers(ru.Body, "Редакция от ")
		enDate := revisionNumbers(en.Body, "Revision of ")
		if ruDate == "" || enDate == "" {
			t.Fatalf("document %q: revision line missing (ru=%q, en=%q)", id, ruDate, enDate)
		}
		if ruDate != enDate {
			t.Errorf("document %q: revision dates differ, ru=%q en=%q; the translation is stale", id, ruDate, enDate)
		}
	}
}

func revisionNumbers(body, prefix string) string {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		var digits []string
		var current strings.Builder
		for _, r := range line {
			if r >= '0' && r <= '9' {
				current.WriteRune(r)
				continue
			}
			if current.Len() > 0 {
				digits = append(digits, current.String())
				current.Reset()
			}
		}
		if current.Len() > 0 {
			digits = append(digits, current.String())
		}
		return strings.Join(digits, "-")
	}
	return ""
}

func headings(body string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") {
			n++
		}
	}
	return n
}
