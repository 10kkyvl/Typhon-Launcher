package platform

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeRejectsEmpty(t *testing.T) {
	for _, path := range []string{"", "   "} {
		if _, err := Normalize(path); !errors.Is(err, ErrEmptyPath) {
			t.Fatalf("Normalize(%q) error = %v, want ErrEmptyPath", path, err)
		}
		if _, err := PathKey(path); !errors.Is(err, ErrEmptyPath) {
			t.Fatalf("PathKey(%q) error = %v, want ErrEmptyPath", path, err)
		}
	}
}

func TestNormalizeMakesPathAbsolute(t *testing.T) {
	normalized, err := Normalize(filepath.Join("games", "one", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(normalized) {
		t.Fatalf("Normalize = %q, want absolute path", normalized)
	}
	if filepath.Base(normalized) != "games" {
		t.Fatalf("Normalize = %q, want the cleaned tail", normalized)
	}
}

func TestInsideContainment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Games")
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"сам корень", root, true},
		{"прямой потомок", filepath.Join(root, "Game"), true},
		{"вложенный файл", filepath.Join(root, "Game", "bin", "game.exe"), true},
		{"сосед с общим префиксом", root + "Other", false},
		{"выход через родителя", filepath.Join(root, "..", "Other"), false},
		{"пустой путь", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Inside(root, tc.path); got != tc.want {
				t.Fatalf("Inside(%q, %q) = %v, want %v", root, tc.path, got, tc.want)
			}
		})
	}
}

func TestInsideEmptyRoot(t *testing.T) {
	if Inside("", filepath.Join(t.TempDir(), "Game")) {
		t.Fatal("пустой корень не содержит ничего")
	}
}

func TestSamePathIgnoresCaseOnWindows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Games", "Portal")
	upper := filepath.Join(filepath.Dir(root), "PORTAL")
	got := SamePath(root, upper)
	want := runtime.GOOS == "windows"
	if got != want {
		t.Fatalf("SamePath(%q, %q) = %v, want %v", root, upper, got, want)
	}
	if !SamePath(root, filepath.Join(root, ".")) {
		t.Fatal("один и тот же путь должен совпадать сам с собой")
	}
}

func TestInsideIgnoresCaseOnWindows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Games")
	nested := filepath.Join(root, "Portal", "game.exe")
	if !Inside(root, nested) {
		t.Fatal("вложенный путь должен считаться внутренним")
	}
	if runtime.GOOS == "windows" && !Inside(root, filepath.Join(filepath.Dir(root), "GAMES", "Portal")) {
		t.Fatal("на Windows сравнение путей регистронезависимо")
	}
}
