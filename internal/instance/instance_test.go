package instance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeAPI struct {
	held       bool
	heldErr    error
	appearsAt  int
	findCalls  int
	findErr    error
	sendErr    error
	sentTo     uintptr
	sentID     uintptr
	sentData   string
	asked      []string
	askedTitle string
}

func (f *fakeAPI) MutexExists(name string) (bool, error) {
	f.asked = append(f.asked, name)
	return f.held, f.heldErr
}

func (f *fakeAPI) FindWindow(class, title string) (uintptr, error) {
	f.asked = append(f.asked, class)
	f.askedTitle = title
	f.findCalls++
	if f.findErr != nil {
		return 0, f.findErr
	}
	if f.appearsAt > 0 && f.findCalls >= f.appearsAt {
		return 77, nil
	}
	return 0, nil
}

func (f *fakeAPI) Send(window uintptr, dataID uintptr, payload string) error {
	f.sentTo, f.sentID, f.sentData = window, dataID, payload
	return f.sendErr
}

type clock struct {
	waited time.Duration
	err    error
}

func (c *clock) wait(_ context.Context, d time.Duration) error {
	c.waited += d
	return c.err
}

func TestForward(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		api       fakeAPI
		clock     clock
		id        string
		want      bool
		wantErr   error
		wantSent  bool
		minWaited time.Duration
	}{
		{name: "no mutex means no first instance", api: fakeAPI{}, id: "a"},
		{name: "window found is forwarded", api: fakeAPI{held: true, appearsAt: 1}, id: "a", want: true, wantSent: true},
		{name: "window appears while the first instance starts", api: fakeAPI{held: true, appearsAt: 5}, id: "a", want: true, wantSent: true, minWaited: 4 * pollEvery},
		{name: "window never appears falls back to a normal start", api: fakeAPI{held: true}, id: "a", minWaited: windowWait},
		{name: "mutex lookup error", api: fakeAPI{heldErr: boom}, id: "a", wantErr: boom},
		{name: "window lookup error", api: fakeAPI{held: true, findErr: boom}, id: "a", wantErr: boom},
		{name: "send error", api: fakeAPI{held: true, appearsAt: 1, sendErr: boom}, id: "a", wantErr: boom, wantSent: true},
		{name: "empty id", id: "", wantErr: ErrNoID},
		{name: "cancelled while waiting", api: fakeAPI{held: true}, clock: clock{err: context.Canceled}, id: "a", wantErr: context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := forward(t.Context(), &tc.api, tc.clock.wait, tc.id, []string{"typhon.exe", "--open", "library"}, `C:\work`)
			if got != tc.want {
				t.Fatalf("forwarded = %v, want %v", got, tc.want)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if tc.wantSent != (tc.api.sentTo != 0) {
				t.Fatalf("sent = %v, want %v", tc.api.sentTo != 0, tc.wantSent)
			}
			if tc.clock.waited < tc.minWaited {
				t.Fatalf("waited %v, want at least %v", tc.clock.waited, tc.minWaited)
			}
		})
	}
}

func TestForwardSendsWailsPayload(t *testing.T) {
	api := &fakeAPI{held: true, appearsAt: 1}
	c := &clock{}
	args := []string{"typhon.exe", "--play", "g1"}
	if _, err := forward(t.Context(), api, c.wait, "com.typhon.launcher", args, `C:\work`); err != nil {
		t.Fatal(err)
	}
	if api.sentID != 1542 {
		t.Fatalf("data id = %d, want 1542", api.sentID)
	}
	var got struct {
		Args       []string `json:"args"`
		WorkingDir string   `json:"workingDir"`
	}
	if err := json.Unmarshal([]byte(api.sentData), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Args) != 3 || got.Args[2] != "g1" || got.WorkingDir != `C:\work` {
		t.Fatalf("payload = %s", api.sentData)
	}
	if api.asked[0] != "wails-app-com.typhon.launcher-sim" || api.asked[1] != "wails-app-com.typhon.launcher-sic" || api.askedTitle != "wails-app-com.typhon.launcher-siw" {
		t.Fatalf("names = %v %q", api.asked, api.askedTitle)
	}
}

func wailsSource(t *testing.T, rel string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "go", "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v3").Output()
	if err != nil {
		t.Fatalf("locate the wails module: %v", err)
	}
	data, err := os.ReadFile(strings.TrimSpace(string(out)) + "/" + rel)
	if err != nil {
		t.Fatalf("read wails source: %v", err)
	}
	return string(data)
}

func TestProtocolMatchesWails(t *testing.T) {
	lock := wailsSource(t, "pkg/application/single_instance_windows.go")
	for name, re := range map[string]string{
		namePrefix:   `id := "` + regexp.QuoteMeta(namePrefix) + `" \+ uniqueID`,
		classSuffix:  `l\.className = id \+ "` + regexp.QuoteMeta(classSuffix) + `"`,
		windowSuffix: `l\.windowName = id \+ "` + regexp.QuoteMeta(windowSuffix) + `"`,
		mutexSuffix:  `mutexName := id \+ "` + regexp.QuoteMeta(mutexSuffix) + `"`,
	} {
		if !regexp.MustCompile(re).MatchString(lock) {
			t.Errorf("wails no longer builds %q the way internal/instance does", name)
		}
	}
	data := wailsSource(t, "pkg/application/single_instance.go")
	for _, field := range []string{`json:"args"`, `json:"workingDir"`} {
		if !strings.Contains(data, field) {
			t.Errorf("wails SecondInstanceData lost %s", field)
		}
	}
	m := regexp.MustCompile(`WMCOPYDATA_SINGLE_INSTANCE_DATA\s*=\s*(\d+)`).FindStringSubmatch(wailsSource(t, "pkg/w32/window.go"))
	if m == nil {
		t.Fatal("wails no longer defines WMCOPYDATA_SINGLE_INSTANCE_DATA")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if n != copyDataID {
		t.Errorf("wails copy data id = %d, internal/instance uses %d", n, copyDataID)
	}
}
