package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWorkerSpecRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	want := workerSpec{
		ID: "abc123", InstallerPath: `C:\downloads\setup.exe`, Engine: EngineInno,
		Destination: `C:\Games\Foo`, WorkingDir: `C:\downloads`, LogPath: `C:\log\i.log`,
		InfPath: `C:\log\i.inf`, StatePath: `C:\log\i-state.json`, CancelPath: `C:\log\i-cancel`,
		Options: installOptions{SkipShortcuts: true, SkipExtras: true}, Background: true, Hidden: true,
	}
	if err := writeWorkerSpec(path, want); err != nil {
		t.Fatalf("writeWorkerSpec: %v", err)
	}
	got, err := readWorkerSpec(path)
	if err != nil {
		t.Fatalf("readWorkerSpec: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readWorkerSpec = %+v, want %+v", got, want)
	}
}

func TestReadWorkerSpecMissingFile(t *testing.T) {
	_, err := readWorkerSpec(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("readWorkerSpec on missing file returned nil error")
	}
}

func TestReadWorkerSpecCorruptJSONKeepsPartialFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.json")
	statePath := filepath.Join(dir, "state.json")
	raw := fmt.Sprintf(`{"statePath": %q, "options": "not-an-object"}`, statePath)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write raw spec: %v", err)
	}
	spec, err := readWorkerSpec(path)
	if err == nil {
		t.Fatal("readWorkerSpec on corrupt json returned nil error")
	}
	if spec.StatePath != statePath {
		t.Fatalf("spec.StatePath = %q, want %q (partial decode lost)", spec.StatePath, statePath)
	}
}

func TestWriteWorkerSpecEmptyPath(t *testing.T) {
	if err := writeWorkerSpec("", workerSpec{}); err == nil {
		t.Fatal("writeWorkerSpec(\"\", ...) returned nil error")
	}
}

func TestWorkerStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "state.json")
	want := workerState{PID: 4242, Phase: string(workerPhaseInstalling), Code: 3010, Done: true, Components: []string{"a", "b"}}
	if err := writeWorkerState(path, want); err != nil {
		t.Fatalf("writeWorkerState: %v", err)
	}
	got, found, err := readWorkerState(path)
	if err != nil {
		t.Fatalf("readWorkerState: %v", err)
	}
	if !found {
		t.Fatal("readWorkerState found = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readWorkerState = %+v, want %+v", got, want)
	}
}

func TestReadWorkerStateMissingFileIsEmptyNotError(t *testing.T) {
	state, found, err := readWorkerState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("readWorkerState missing file error = %v, want nil", err)
	}
	if found {
		t.Fatal("readWorkerState found = true for missing file")
	}
	if !reflect.DeepEqual(state, workerState{}) {
		t.Fatalf("readWorkerState state = %+v, want zero value", state)
	}
}

func TestReadWorkerStateCorruptJSONIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write raw state: %v", err)
	}
	_, found, err := readWorkerState(path)
	if err == nil {
		t.Fatal("readWorkerState on corrupt json returned nil error")
	}
	if found {
		t.Fatal("readWorkerState found = true on corrupt json")
	}
}

func TestReadWorkerStateEmptyPath(t *testing.T) {
	_, _, err := readWorkerState("")
	if !errors.Is(err, errWorkerStatePathUnavailable) {
		t.Fatalf("readWorkerState(\"\") error = %v, want errWorkerStatePathUnavailable", err)
	}
}

func TestWriteWorkerStateEmptyPath(t *testing.T) {
	if err := writeWorkerState("", workerState{}); !errors.Is(err, errWorkerStatePathUnavailable) {
		t.Fatalf("writeWorkerState(\"\", ...) error = %v, want errWorkerStatePathUnavailable", err)
	}
}

func TestWorkerCancelMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "cancel")

	if workerCancelRequested(path) {
		t.Fatal("workerCancelRequested = true before marker created")
	}
	if err := writeWorkerCancel(path); err != nil {
		t.Fatalf("writeWorkerCancel: %v", err)
	}
	if !workerCancelRequested(path) {
		t.Fatal("workerCancelRequested = false after marker created")
	}
	if err := clearWorkerCancel(path); err != nil {
		t.Fatalf("clearWorkerCancel: %v", err)
	}
	if workerCancelRequested(path) {
		t.Fatal("workerCancelRequested = true after marker cleared")
	}
	if err := clearWorkerCancel(path); err != nil {
		t.Fatalf("clearWorkerCancel on already-missing marker: %v", err)
	}
}

func TestClearWorkerCancelPropagatesRemoveFailure(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file inside dir: %v", err)
	}
	if err := clearWorkerCancel(blocked); err == nil {
		t.Fatal("clearWorkerCancel on non-empty directory returned nil error")
	}
}

func TestWatchWorkerCancelCancelsContext(t *testing.T) {
	restore := workerCancelPollInterval
	workerCancelPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { workerCancelPollInterval = restore })

	dir := t.TempDir()
	path := filepath.Join(dir, "cancel")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		watchWorkerCancel(ctx, path, cancel)
		close(done)
	}()

	if err := writeWorkerCancel(path); err != nil {
		t.Fatalf("writeWorkerCancel: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("ctx was not cancelled after cancel marker appeared")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchWorkerCancel did not return after cancelling ctx")
	}
}

func TestWatchWorkerCancelReturnsImmediatelyWithoutPath(t *testing.T) {
	done := make(chan struct{})
	go func() {
		watchWorkerCancel(context.Background(), "", func() {})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchWorkerCancel(\"\") did not return")
	}
}

func TestDiscoverComponentsSkipsWhenNotInno(t *testing.T) {
	in := discoverySpec{Engine: EngineNsis, Options: installOptions{SkipExtras: true, SkipShortcuts: true}}
	components, reason, err := discoverComponents(context.Background(), in)
	if err != nil || reason != "" || components != nil {
		t.Fatalf("discoverComponents = (%v, %q, %v), want (nil, \"\", nil)", components, reason, err)
	}
}

func TestDiscoverComponentsSkipsWhenNoOptionsRequested(t *testing.T) {
	in := discoverySpec{Engine: EngineInno}
	components, reason, err := discoverComponents(context.Background(), in)
	if err != nil || reason != "" || components != nil {
		t.Fatalf("discoverComponents = (%v, %q, %v), want (nil, \"\", nil)", components, reason, err)
	}
}

func TestShouldDiscoverComponents(t *testing.T) {
	cases := []struct {
		name string
		in   discoverySpec
		want bool
	}{
		{"inno with skip extras", discoverySpec{Engine: EngineInno, Options: installOptions{SkipExtras: true}}, true},
		{"inno with skip shortcuts", discoverySpec{Engine: EngineInno, Options: installOptions{SkipShortcuts: true}}, true},
		{"inno without options", discoverySpec{Engine: EngineInno}, false},
		{"nsis with both options", discoverySpec{Engine: EngineNsis, Options: installOptions{SkipExtras: true, SkipShortcuts: true}}, false},
		{"msi with both options", discoverySpec{Engine: EngineMsi, Options: installOptions{SkipExtras: true, SkipShortcuts: true}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldDiscoverComponents(tc.in); got != tc.want {
				t.Fatalf("shouldDiscoverComponents(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestApplyDiscoveredComponentsAddsComponentsFlag(t *testing.T) {
	dir := t.TempDir()
	setup := filepath.Join(dir, "setup.exe")
	spec := runSpec{
		Path: setup, InstallerPath: setup, Engine: EngineInno,
		Destination: filepath.Join(dir, "Foo"), Args: []string{"placeholder"},
	}
	got, err := applyDiscoveredComponents(spec, []string{"compgame", "lang"})
	if err != nil {
		t.Fatalf("applyDiscoveredComponents error = %v", err)
	}
	want := "/COMPONENTS=compgame,lang"
	found := false
	for _, a := range got.Args {
		if a == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("Args = %v, want to contain %q", got.Args, want)
	}
}

func TestApplyDiscoveredComponentsNoopWithoutComponents(t *testing.T) {
	dir := t.TempDir()
	setup := filepath.Join(dir, "setup.exe")
	spec := runSpec{Path: setup, InstallerPath: setup, Engine: EngineInno, Destination: filepath.Join(dir, "Foo"), Args: []string{"orig"}}
	got, err := applyDiscoveredComponents(spec, nil)
	if err != nil {
		t.Fatalf("applyDiscoveredComponents error = %v", err)
	}
	if !reflect.DeepEqual(got, spec) {
		t.Fatalf("applyDiscoveredComponents modified spec without components: %+v", got)
	}
}

func TestApplyDiscoveredComponentsPropagatesBuildError(t *testing.T) {
	setup := filepath.Join(t.TempDir(), "setup.exe")
	spec := runSpec{Path: setup, InstallerPath: setup, Engine: EngineInno}
	if _, err := applyDiscoveredComponents(spec, []string{"compgame"}); err == nil {
		t.Fatal("applyDiscoveredComponents with empty destination returned nil error")
	}
}

func TestRunWorkerRecordsStateWhenSpecStatePathIsKnown(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.json")
	statePath := filepath.Join(dir, "state.json")
	raw := fmt.Sprintf(`{"statePath": %q, "options": "not-an-object"}`, statePath)
	if err := os.WriteFile(specPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write raw spec: %v", err)
	}

	if err := RunWorker(specPath); err == nil {
		t.Fatal("RunWorker on a broken spec returned nil error")
	}

	state, found, err := readWorkerState(statePath)
	if err != nil {
		t.Fatalf("readWorkerState: %v", err)
	}
	if !found || !state.Done || state.Error == "" {
		t.Fatalf("state = %+v, want a done state carrying the spec error", state)
	}
}

func TestRunWorkerReturnsErrorWhenSpecUnreadable(t *testing.T) {
	if err := RunWorker(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("RunWorker on a missing spec file returned nil error")
	}
}
