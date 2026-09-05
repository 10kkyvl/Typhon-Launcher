package uierr_test

import (
	"errors"
	"fmt"
	"testing"

	"typhon/internal/uierr"
)

var errGameRunning = uierr.New("relocate.game_running", "игра сейчас запущена")

func TestErrorCarriesCodeAndDetail(t *testing.T) {
	got := errGameRunning.Error()
	want := "typhon:relocate.game_running: игра сейчас запущена"
	if got != want {
		t.Fatalf("текст ошибки %q, ожидался %q", got, want)
	}
}

func TestSentinelStaysComparable(t *testing.T) {
	wrapped := fmt.Errorf("перенос не начат: %w", errGameRunning)
	if !errors.Is(wrapped, errGameRunning) {
		t.Fatal("errors.Is перестал узнавать сентинел после обёртки")
	}
}

func TestCodeReadsThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("перенос не начат: %w", errGameRunning)
	if got := uierr.Code(wrapped); got != "relocate.game_running" {
		t.Fatalf("код %q, ожидался relocate.game_running", got)
	}
}

func TestCodeIsEmptyForPlainErrors(t *testing.T) {
	if got := uierr.Code(errors.New("что-то пошло не так")); got != "" {
		t.Fatalf("код %q, ожидался пустой", got)
	}
	if got := uierr.Code(nil); got != "" {
		t.Fatalf("код %q для nil, ожидался пустой", got)
	}
}

func TestWrapAddsACodeToAnExistingError(t *testing.T) {
	cause := errors.New("disk full")
	err := uierr.Wrap("install.disk_full", cause)
	if !errors.Is(err, cause) {
		t.Fatal("обёртка потеряла исходную ошибку")
	}
	if got := uierr.Code(err); got != "install.disk_full" {
		t.Fatalf("код %q, ожидался install.disk_full", got)
	}
	if got := err.Error(); got != "typhon:install.disk_full: disk full" {
		t.Fatalf("текст %q", got)
	}
}

func TestInnermostCodeWins(t *testing.T) {
	inner := uierr.New("install.disk_full", "нет места")
	outer := uierr.Wrap("install.failed", inner)
	if got := uierr.Code(outer); got != "install.failed" {
		t.Fatalf("код %q, ожидался install.failed", got)
	}
}
