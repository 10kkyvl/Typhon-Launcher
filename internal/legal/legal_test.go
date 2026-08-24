package legal

import (
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func validFS() fstest.MapFS {
	return fstest.MapFS{
		"TERMS.md":               {Data: []byte("# Условия использования")},
		"PRIVACY.md":             {Data: []byte("# Политика конфиденциальности")},
		"COPYRIGHT.md":           {Data: []byte("# Авторские права")},
		"THIRD_PARTY_NOTICES.md": {Data: []byte("# Лицензии сторонних компонентов")},
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
			doc, err := svc.Get(meta.ID)
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
			doc, err := svc.Get(id)
			if err == nil {
				t.Fatalf("Get(%q): expected error, got nil (doc=%+v)", id, doc)
			}
			if !errors.Is(err, ErrUnknownDocument) {
				t.Fatalf("Get(%q): error = %v, want wrapping ErrUnknownDocument", id, err)
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

	for _, meta := range Required {
		doc, err := svc.Get(meta.ID)
		if err != nil {
			t.Fatalf("Get(%q): unexpected error: %v", meta.ID, err)
		}
		lower := strings.ToLower(doc.Body)
		for _, marker := range devAddressMarkers {
			if strings.Contains(lower, strings.ToLower(marker)) {
				t.Errorf("document %q contains dev address marker %q", meta.ID, marker)
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

	for _, meta := range Required {
		doc, err := svc.Get(meta.ID)
		if err != nil {
			t.Fatalf("Get(%q): unexpected error: %v", meta.ID, err)
		}
		if strings.TrimSpace(doc.Body) == "" {
			t.Fatalf("real document %q is empty", meta.ID)
		}
	}
}
