package accountsync

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func startedHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	if err := h.service.ServiceStartup(t.Context(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		if err := h.service.ServiceShutdown(); err != nil {
			t.Fatalf("ServiceShutdown: %v", err)
		}
	})
	return h
}

func seedState(t *testing.T, h *harness) syncState {
	t.Helper()
	h.server.get = func(w http.ResponseWriter) {
		writeJSON(w, http.StatusOK, snapshotBody{Games: []wireGame{}})
	}
	h.server.put = echoPut(http.StatusOK)
	if err := h.service.Sync(context.Background()); err != nil {
		t.Fatalf("seed Sync: %v", err)
	}
	st := h.readState()
	if st.DeviceID == "" {
		t.Fatal("seed produced no device id")
	}
	return st
}

func TestForgetRemoteWipesServerStateAndToggle(t *testing.T) {
	h := startedHarness(t)
	seedState(t, h)

	if err := h.service.ForgetRemote(); err != nil {
		t.Fatalf("ForgetRemote: %v", err)
	}

	if h.server.delCalls != 1 {
		t.Fatalf("expected exactly one DELETE, got %d", h.server.delCalls)
	}
	after := h.readState()
	if after.DeviceID != "" || after.SettingsRevision != 0 || len(after.Games) != 0 {
		t.Fatalf("state survived the wipe: %+v", after)
	}
	if h.settings.value.AccountSync {
		t.Error("синхронизация осталась включённой после удаления данных: следующий цикл зальёт всё обратно")
	}
}

func TestForgetRemoteKeepsStateWhenServerFails(t *testing.T) {
	h := startedHarness(t)
	before := seedState(t, h)
	h.server.del = func(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError) }

	err := h.service.ForgetRemote()
	if err == nil {
		t.Fatal("expected an error when the server refuses to delete")
	}
	var serverErr *ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("expected a server error, got %v", err)
	}

	after := h.readState()
	if !stateEqual(before, after) {
		t.Errorf("состояние изменилось при неудачном удалении: было %+v, стало %+v", before, after)
	}
	if !h.settings.value.AccountSync {
		t.Error("синхронизация выключена, хотя данные на сервере остались")
	}
}

func TestForgetRemoteBeforeStartup(t *testing.T) {
	h := newHarness(t)
	if err := h.service.ForgetRemote(); !errors.Is(err, errNotStarted) {
		t.Fatalf("expected errNotStarted, got %v", err)
	}
	if h.server.delCalls != 0 {
		t.Fatalf("expected no DELETE before startup, got %d", h.server.delCalls)
	}
}
