package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

var (
	tsFieldPattern = regexp.MustCompile(`(?m)^\s+(\w+)\??:`)
	tsUnionPattern = regexp.MustCompile(`'([a-z]+)'`)
)

func readSelfupdateTS(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "frontend", "src", "lib", "services", "selfupdate.ts")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("frontend contract file not readable: %v", err)
	}
	return string(body)
}

func tsInterfaceKeys(t *testing.T, ts, name string) []string {
	t.Helper()
	block := regexp.MustCompile(`(?s)export interface ` + name + ` \{(.*?)\n\}`).FindStringSubmatch(ts)
	if block == nil {
		t.Fatalf("interface %s not found in selfupdate.ts", name)
	}
	var keys []string
	for _, m := range tsFieldPattern.FindAllStringSubmatch(block[1], -1) {
		keys = append(keys, m[1])
	}
	sort.Strings(keys)
	return keys
}

func tsUnionValues(t *testing.T, ts, name string) []string {
	t.Helper()
	decl := regexp.MustCompile(`export type ` + name + ` =([^;]*);`).FindStringSubmatch(ts)
	if decl == nil {
		t.Fatalf("type %s not found in selfupdate.ts", name)
	}
	var values []string
	for _, m := range tsUnionPattern.FindAllStringSubmatch(decl[1], -1) {
		values = append(values, m[1])
	}
	sort.Strings(values)
	return values
}

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal %T: %v", v, err)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestJSONShapesMatchTheFrontendInterfaces(t *testing.T) {
	ts := readSelfupdateTS(t)
	stamp := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	change := Change{Kind: ChangeFixed, Text: "fixed"}
	note := ReleaseNote{Version: "1.2.3", PublishedAt: stamp, Summary: "s", Changes: []Change{change}}

	tests := []struct {
		name  string
		value any
		iface string
	}{
		{"status", Status{
			State: StateReady, CurrentVersion: "1.0.0", AvailableVersion: "1.2.3", Notes: "n", PublishedAt: stamp,
			TotalBytes: 1, DownloadedBytes: 1, CheckedAt: stamp, Error: "e", ErrorCode: "c",
		}, "SelfUpdateStatus"},
		{"progress", Progress{Version: "1.2.3", TotalBytes: 2, DownloadedBytes: 1}, "SelfUpdateProgress"},
		{"outcome", Outcome{Version: "1.2.3", OK: true, Error: "e", FinishedAt: stamp}, "SelfUpdateOutcome"},
		{"release note", note, "ReleaseNote"},
		{"release change", change, "ReleaseChange"},
		{"release notes", ReleaseNotes{CurrentVersion: "1.0.0", Unseen: []ReleaseNote{note}, History: []ReleaseNote{note}}, "ReleaseNotes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, want := jsonKeys(t, tt.value), tsInterfaceKeys(t, ts, tt.iface)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Go sends %v, interface %s reads %v: a field renamed on one side arrives as undefined on the other", got, tt.iface, want)
			}
		})
	}
}

func TestEnumValuesMatchTheFrontendUnions(t *testing.T) {
	ts := readSelfupdateTS(t)

	states := []string{
		string(StateIdle), string(StateChecking), string(StateAvailable), string(StateDownloading),
		string(StateReady), string(StateApplying), string(StateFailed),
	}
	sort.Strings(states)
	if got := tsUnionValues(t, ts, "SelfUpdateState"); !reflect.DeepEqual(got, states) {
		t.Fatalf("Go states %v, frontend SelfUpdateState %v", states, got)
	}

	kinds := []string{string(ChangeAdded), string(ChangeChanged), string(ChangeFixed), string(ChangeRemoved)}
	sort.Strings(kinds)
	if got := tsUnionValues(t, ts, "ReleaseChangeKind"); !reflect.DeepEqual(got, kinds) {
		t.Fatalf("Go change kinds %v, frontend ReleaseChangeKind %v", kinds, got)
	}
}

func TestManifestWireFormat(t *testing.T) {
	priv, pub := testKeyPair(t)
	payload := []byte(`{"version":"1.2.3","publishedAt":"2026-09-02T12:00:00Z","notes":"fixes",` +
		`"releases":[{"version":"1.2.3","publishedAt":"2026-09-02T12:00:00Z","summary":"s","changes":[{"kind":"fixed","text":"t"}]}],` +
		`"artifacts":[{"os":"windows","arch":"amd64","kind":"installer","name":"typhon-setup.exe",` +
		`"url":"https://cdn.example.com/typhon-setup.exe","size":1024,"sha256":"` + strings.Repeat("ab", 32) + `"}]}`)

	m, err := VerifyManifest(envelopeJSON(t, priv, payload), pub)
	if err != nil {
		t.Fatalf("VerifyManifest() error = %v: the release tooling's document is no longer readable", err)
	}
	art, err := m.ArtifactFor("windows", "amd64")
	if err != nil {
		t.Fatalf("ArtifactFor: %v", err)
	}
	want := Artifact{
		OS: "windows", Arch: "amd64", Kind: KindInstaller, Name: "typhon-setup.exe",
		URL: "https://cdn.example.com/typhon-setup.exe", Size: 1024, SHA256: strings.Repeat("ab", 32),
	}
	if art != want {
		t.Fatalf("artifact = %+v, want %+v", art, want)
	}
	if m.Version != "1.2.3" || m.Notes != "fixes" || len(m.Releases) != 1 || m.Releases[0].Changes[0].Kind != ChangeFixed {
		t.Fatalf("manifest = %+v", m)
	}

	signed, err := SignManifest(m, priv)
	if err != nil {
		t.Fatalf("SignManifest: %v", err)
	}
	var env SignedManifest
	if err := json.Unmarshal(signed, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if got, wantKeys := jsonKeys(t, env), []string{"keyId", "manifest", "signature"}; !reflect.DeepEqual(got, wantKeys) {
		t.Fatalf("envelope keys = %v, want %v", got, wantKeys)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(env.Manifest, &fields); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if wantKeys := []string{"artifacts", "notes", "publishedAt", "releases", "version"}; !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("manifest keys = %v, want %v", keys, wantKeys)
	}
	var artifacts []map[string]json.RawMessage
	if err := json.Unmarshal(fields["artifacts"], &artifacts); err != nil || len(artifacts) != 1 {
		t.Fatalf("artifacts = %v, %v", artifacts, err)
	}
	var artKeys []string
	for k := range artifacts[0] {
		artKeys = append(artKeys, k)
	}
	sort.Strings(artKeys)
	if wantKeys := []string{"arch", "kind", "name", "os", "sha256", "size", "url"}; !reflect.DeepEqual(artKeys, wantKeys) {
		t.Fatalf("artifact keys = %v, want %v", artKeys, wantKeys)
	}
}

func TestStateFileFormatIsStable(t *testing.T) {
	dir := t.TempDir()
	path := stateFilePath(t, dir)
	hash := strings.Repeat("ab", 32)
	writeTestFile(t, path, []byte(`{"version":1,"data":{"availableVersion":"1.2.3","notes":"n",`+
		`"publishedAt":"2026-09-02T12:00:00Z","checkedAt":"2026-09-02T12:05:00Z",`+
		`"artifact":{"os":"windows","arch":"amd64","kind":"installer","name":"typhon-setup.exe",`+
		`"url":"https://cdn.example.com/typhon-setup.exe","size":1024,"sha256":"`+hash+`"},`+
		`"readyPath":"C:\\cache\\1.2.3\\typhon-setup.exe"}}`))

	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v: a state written by an earlier launcher is no longer readable", err)
	}
	if got.AvailableVersion != "1.2.3" || got.Notes != "n" || got.ReadyPath != `C:\cache\1.2.3\typhon-setup.exe` {
		t.Fatalf("Load() = %+v", got)
	}
	if !got.PublishedAt.Equal(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)) || !got.CheckedAt.Equal(time.Date(2026, 9, 2, 12, 5, 0, 0, time.UTC)) {
		t.Fatalf("Load() times = %v, %v", got.PublishedAt, got.CheckedAt)
	}
	if got.Artifact == nil || got.Artifact.Size != 1024 || got.Artifact.SHA256 != hash || got.Artifact.Kind != KindInstaller || got.Artifact.Name != "typhon-setup.exe" {
		t.Fatalf("Load() artifact = %+v", got.Artifact)
	}

	if err := s.Save(got); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var envelope struct {
		Version int                        `json:"version"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	var keys []string
	for k := range envelope.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if envelope.Version != StateVersion || !reflect.DeepEqual(keys, []string{"artifact", "availableVersion", "checkedAt", "notes", "publishedAt", "readyPath"}) {
		t.Fatalf("state written as version %d with keys %v: another launcher version reads this file", envelope.Version, keys)
	}
}

func TestWorkerRecordsKeepTheirFormat(t *testing.T) {
	t.Run("outcome left by the previous launcher's worker", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "last-update.json")
		writeTestFile(t, path, []byte(`{"version":1,"data":{"version":"1.2.3","ok":false,"error":"boom","finishedAt":"2026-09-02T12:30:00Z"}}`))
		got, err := readOutcome(path)
		if err != nil {
			t.Fatalf("readOutcome() error = %v: the relaunched launcher could not report how the update went", err)
		}
		if got.Version != "1.2.3" || got.OK || got.Error != "boom" || !got.FinishedAt.Equal(time.Date(2026, 9, 2, 12, 30, 0, 0, time.UTC)) {
			t.Fatalf("readOutcome() = %+v", got)
		}

		if err := writeOutcome(path, got); err != nil {
			t.Fatalf("writeOutcome: %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read outcome: %v", err)
		}
		var envelope struct {
			Version int                        `json:"version"`
			Data    map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal outcome: %v", err)
		}
		var keys []string
		for k := range envelope.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if envelope.Version != outcomeVersion || !reflect.DeepEqual(keys, []string{"error", "finishedAt", "ok", "version"}) {
			t.Fatalf("outcome written as version %d with keys %v: the next launcher reads this file", envelope.Version, keys)
		}
	})

	t.Run("update spec", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "update-spec.json")
		writeTestFile(t, path, []byte(`{"language":"en","installerPath":"C:\\cache\\setup.exe","installDir":"C:\\Program Files\\Typhon",`+
			`"parentPid":4242,"relaunchPath":"C:\\Program Files\\Typhon\\typhon.exe","version":"1.2.3"}`))
		got, err := readUpdateSpec(path)
		if err != nil {
			t.Fatalf("readUpdateSpec() error = %v", err)
		}
		want := updateSpec{
			Language: "en", InstallerPath: `C:\cache\setup.exe`, InstallDir: `C:\Program Files\Typhon`,
			ParentPID: 4242, RelaunchPath: `C:\Program Files\Typhon\typhon.exe`, Version: "1.2.3",
		}
		if got != want {
			t.Fatalf("readUpdateSpec() = %+v, want %+v", got, want)
		}
	})
}
