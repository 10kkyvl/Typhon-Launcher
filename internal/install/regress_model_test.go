package install

import (
	"path/filepath"
	"testing"
	"time"

	"typhon/internal/download"
)

// Группы статусов решают, что можно отменить, повторить и убрать из списка; новый
// статус без места в таблице молча попал бы не в ту группу.
func TestEveryStatusBelongsToExactlyOneGroup(t *testing.T) {
	cases := []struct {
		status    Status
		transient bool
		active    bool
		retryable bool
	}{
		{StatusPending, true, true, false},
		{StatusPreparing, true, true, false},
		{StatusInstalling, true, true, false},
		{StatusExtracting, true, true, false},
		{StatusVerifying, true, true, false},
		{StatusWaitingForUser, false, true, false},
		{StatusCompleted, false, false, false},
		{StatusFailed, false, false, true},
		{StatusCancelled, false, false, true},
		{StatusInterrupted, false, false, true},
	}
	if declared := goConsts(t, "model.go", ofType("Status")); len(declared) != len(cases) {
		t.Fatalf("model.go объявляет %d статусов, таблица знает %d: новому статусу нужна строка", len(declared), len(cases))
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := transient(tc.status); got != tc.transient {
				t.Errorf("transient = %v, want %v", got, tc.transient)
			}
			if got := active(tc.status); got != tc.active {
				t.Errorf("active = %v, want %v", got, tc.active)
			}
			if got := retryable(tc.status); got != tc.retryable {
				t.Errorf("retryable = %v, want %v", got, tc.retryable)
			}
			if tc.active && tc.retryable {
				t.Error("a status that is both running and retryable would allow a second job over the first")
			}
		})
	}
}

func TestTypeGroups(t *testing.T) {
	cases := []struct {
		typ         Type
		controlled  bool
		archived    bool
		external    bool
		ownDestSlnt bool
		ownDestWiz  bool
	}{
		{TypePortable, true, false, false, true, true},
		{TypeArchiveZip, true, true, false, true, true},
		{TypeArchive7z, true, true, false, true, true},
		{TypeArchiveRar, true, true, false, true, true},
		{TypeExeInstaller, false, false, true, true, false},
		{TypeMsiInstaller, false, false, true, true, false},
		{TypeUnknown, false, false, false, false, false},
	}
	if declared := goConsts(t, "model.go", ofType("Type")); len(declared) != len(cases) {
		t.Fatalf("model.go объявляет %d типов, таблица знает %d: новому типу нужна строка", len(declared), len(cases))
	}
	for _, tc := range cases {
		t.Run(string(tc.typ), func(t *testing.T) {
			if controlled(tc.typ) != tc.controlled || archived(tc.typ) != tc.archived || external(tc.typ) != tc.external {
				t.Errorf("controlled %v archived %v external %v", controlled(tc.typ), archived(tc.typ), external(tc.typ))
			}
			if ownDestination(tc.typ, true) != tc.ownDestSlnt || ownDestination(tc.typ, false) != tc.ownDestWiz {
				t.Errorf("ownDestination silent %v wizard %v", ownDestination(tc.typ, true), ownDestination(tc.typ, false))
			}
		})
	}
}

func TestInstallMode(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		seeding bool
		want    string
	}{
		{"move on request", ModeMove, false, ModeMove},
		{"copy on request", ModeCopy, false, ModeCopy},
		{"nothing asked", "", false, ModeCopy},
		{"garbage asked", "teleport", false, ModeCopy},
		{"seeding forces copy over move", ModeMove, true, ModeCopy},
		{"seeding keeps copy", ModeCopy, true, ModeCopy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := installMode(tc.mode, tc.seeding); got != tc.want {
				t.Fatalf("installMode(%q, %v) = %q, want %q", tc.mode, tc.seeding, got, tc.want)
			}
		})
	}
}

func TestRequiredBytes(t *testing.T) {
	cases := []struct {
		name string
		plan Plan
		want int64
	}{
		{"estimated size gets a five percent margin", Plan{EstimatedSize: 2000}, 2100},
		{"compressed size alone is tripled", Plan{CompressedSize: 100}, 315},
		{"estimate wins over compressed size", Plan{EstimatedSize: 1000, CompressedSize: 999}, 1050},
		{"nothing known needs nothing", Plan{}, 0},
		{"negative estimate falls back to compressed", Plan{EstimatedSize: -1, CompressedSize: 10}, 31},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiredBytes(tc.plan); got != tc.want {
				t.Fatalf("requiredBytes = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:       "0 Б",
		1023:    "1023 Б",
		1 << 10: "1.0 КБ",
		5 << 20: "5.0 МБ",
		3 << 30: "3.0 ГБ",
	}
	for bytes, want := range cases {
		if got := humanSize(bytes); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", bytes, got, want)
		}
	}
}

// Запись, пережившая аварию, считается установленной, только если каталог уже
// заполнен, а .partial исчез: любое другое состояние — настоящее прерывание.
func TestCrashFinalizable(t *testing.T) {
	cases := []struct {
		name  string
		build func(t *testing.T) Installation
		want  bool
	}{
		{"archive committed before the crash", func(t *testing.T) Installation {
			dest := filepath.Join(t.TempDir(), "Game")
			mkFile(t, filepath.Join(dest, "Game.exe"), 8)
			return Installation{Type: TypeArchiveZip, Destination: dest}
		}, true},
		{"portable committed before the crash", func(t *testing.T) Installation {
			dest := filepath.Join(t.TempDir(), "Game")
			mkFile(t, filepath.Join(dest, "Game.exe"), 8)
			return Installation{Type: TypePortable, Destination: dest}
		}, true},
		{"partial still there", func(t *testing.T) Installation {
			dest := filepath.Join(t.TempDir(), "Game")
			mkFile(t, filepath.Join(dest, "Game.exe"), 8)
			mkFile(t, filepath.Join(dest+partialSuffix, "chunk"), 8)
			return Installation{Type: TypeArchiveZip, Destination: dest}
		}, false},
		{"destination missing", func(t *testing.T) Installation {
			return Installation{Type: TypeArchiveZip, Destination: filepath.Join(t.TempDir(), "Game")}
		}, false},
		{"destination empty", func(t *testing.T) Installation {
			dest := filepath.Join(t.TempDir(), "Game")
			mkdirs(t, dest)
			return Installation{Type: TypeArchiveZip, Destination: dest}
		}, false},
		{"no destination recorded", func(t *testing.T) Installation {
			return Installation{Type: TypeArchiveZip}
		}, false},
		{"installer writes into the folder itself", func(t *testing.T) Installation {
			dest := filepath.Join(t.TempDir(), "Game")
			mkFile(t, filepath.Join(dest, "Game.exe"), 8)
			return Installation{Type: TypeExeInstaller, Destination: dest, Silent: true}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := crashFinalizable(tc.build(t)); got != tc.want {
				t.Fatalf("crashFinalizable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSourceDirChoosesWhatTheDownloadFetched(t *testing.T) {
	root := t.TempDir()
	mkFile(t, filepath.Join(root, "Nested", "a.bin"), 4)
	mkFile(t, filepath.Join(root, "loose.zip"), 4)
	cases := []struct {
		name string
		d    download.Download
		want string
		code string
	}{
		{"folder under the name", download.Download{ID: "d", Destination: root, Name: "Nested"}, filepath.Join(root, "Nested"), ""},
		{"single file next to other downloads", download.Download{ID: "d", Destination: root, Name: "loose.zip"}, filepath.Join(root, "loose.zip"), ""},
		{"nothing under the name", download.Download{ID: "d", Destination: root, Name: "Missing"}, root, ""},
		{"flat layout", download.Download{ID: "d", Destination: root, Name: "Nested", Flat: true}, root, ""},
		{"in-place layout", download.Download{ID: "d", Destination: root, Name: "Nested", InPlace: true}, root, ""},
		{"no name", download.Download{ID: "d", Destination: root}, root, ""},
		{"no destination", download.Download{ID: "d", Name: "Nested"}, "", "install.no_source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sourceDir(tc.d)
			if code := errCode(err); code != tc.code {
				t.Fatalf("error = %v (code %q), want code %q", err, code, tc.code)
			}
			if got != tc.want {
				t.Fatalf("sourceDir = %q, want %q", got, tc.want)
			}
		})
	}
}

// Снимок, отданный интерфейсу или другому сервису, не должен делить память с
// записью: иначе чужая правка среза меняет состояние установки без блокировки.
func TestSnapshotDoesNotAliasTheRecord(t *testing.T) {
	done := time.Now()
	item := &Installation{
		ID: "a", Candidates: []Candidate{{Path: "a.exe", Score: 70}}, ExtraInstallers: []string{"x.exe"}, CompletedAt: &done,
	}
	snap := snapshotOf(item)
	snap.Candidates[0].Path = "changed.exe"
	snap.ExtraInstallers[0] = "changed.exe"
	*snap.CompletedAt = done.Add(time.Hour)

	if item.Candidates[0].Path != "a.exe" || item.ExtraInstallers[0] != "x.exe" || !item.CompletedAt.Equal(done) {
		t.Fatalf("editing a snapshot changed the record: %+v", item)
	}
}
