package install

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const secondRun = "second run"

func chainTools(t *testing.T, first archiveTool) {
	t.Helper()
	useTools(t, first, fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": secondRun}}))
}

func secondToolRan(t *testing.T, dest string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dest, "Game", "game.exe"))
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data) == secondRun
}

func TestVerificationFailureStopsTheChain(t *testing.T) {
	errFlush := errors.New("flush failed")
	errBusy := errors.New("sharing violation")
	good := fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}}
	cases := []struct {
		name    string
		archive func(t *testing.T) string
		plan    fakePlan
		setup   func(t *testing.T)
		wantIs  []error
		wantNot []error
	}{
		{
			name:    "file the tool never wrote",
			archive: func(t *testing.T) string { return storedRar(t, brokenGame(), laterEntry("Game/later1.bin")) },
			plan:    good,
			wantIs:  []error{errArchiveMismatch, errFileGone},
			wantNot: []error{errArchiveToolFailed, errArchiveUnsafe},
		},
		{
			name:    "entry that is not in the listing",
			archive: func(t *testing.T) string { return brokenRar(t) },
			plan:    fakePlan{Files: map[string]string{"Game/game.exe": toolPayload, "Game/extra.dll": "x"}},
			wantIs:  []error{errArchiveMismatch, errArchiveUnlisted},
			wantNot: []error{errArchiveToolFailed, errArchiveUnsafe},
		},
		{
			name:    "flush to disk fails",
			archive: func(t *testing.T) string { return brokenRar(t) },
			plan:    good,
			setup: func(t *testing.T) {
				prev := syncExtracted
				syncExtracted = func(string, fs.FileMode) (uint64, error) { return 0, errFlush }
				t.Cleanup(func() { syncExtracted = prev })
			},
			wantIs:  []error{errArchiveVerify, errFlush},
			wantNot: []error{errArchiveToolFailed, errArchiveMismatch},
		},
		{
			name:    "file stays locked by another program",
			archive: func(t *testing.T) string { return brokenRar(t) },
			plan:    good,
			setup: func(t *testing.T) {
				prevSync, prevBusy, prevDelay := syncExtracted, fileBusy, verifyRetryDelay
				syncExtracted = func(string, fs.FileMode) (uint64, error) { return 0, errBusy }
				fileBusy = func(err error) bool { return errors.Is(err, errBusy) }
				verifyRetryDelay = time.Millisecond
				t.Cleanup(func() { syncExtracted, fileBusy, verifyRetryDelay = prevSync, prevBusy, prevDelay })
			},
			wantIs:  []error{errArchiveVerify, errBusy},
			wantNot: []error{errArchiveToolFailed},
		},
		{
			name:    "file disappears while it is being verified",
			archive: func(t *testing.T) string { return brokenRar(t) },
			plan:    good,
			setup: func(t *testing.T) {
				real := syncExtracted
				syncExtracted = func(path string, mode fs.FileMode) (uint64, error) {
					if err := os.Remove(path); err != nil {
						return 0, err
					}
					return real(path, mode)
				}
				t.Cleanup(func() { syncExtracted = real })
			},
			wantIs:  []error{errArchiveMismatch, errFileGone},
			wantNot: []error{errArchiveToolFailed, errArchiveVerify},
		},
		{
			name: "hard link in the result",
			archive: func(t *testing.T) string {
				return brokenRar(t, rarEntry{name: "Game/copy.exe", data: []byte(toolPayload)})
			},
			plan: fakePlan{
				Files:     map[string]string{"Game/game.exe": toolPayload},
				HardLinks: map[string]string{"Game/copy.exe": "Game/game.exe"},
			},
			wantIs:  []error{errArchiveUnsafe, errArchiveHardLinked},
			wantNot: []error{errArchiveToolFailed},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}
			chainTools(t, fakePlanTool(t, tc.plan))
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), tc.archive(t), dest, nil)
			for _, want := range tc.wantIs {
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want errors.Is %v", err, want)
				}
			}
			for _, not := range tc.wantNot {
				if errors.Is(err, not) {
					t.Fatalf("err = %v, must not be %v", err, not)
				}
			}
			if secondToolRan(t, dest) {
				t.Fatal("the second tool ran after a failure of the verification that it cannot fix")
			}
		})
	}
}

func TestSizeMismatchGivesTheNextToolAChance(t *testing.T) {
	useTools(t,
		fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": "short"}}),
		fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}}),
	)
	dest := filepath.Join(t.TempDir(), "out")
	if err := ExtractArchive(context.Background(), brokenRar(t), dest, nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "Game", "game.exe"))
	if err != nil || string(data) != toolPayload {
		t.Fatalf("game.exe = %q, %v: the second tool did not run", data, err)
	}
}

var errBusyFile = errors.New("sharing violation")

func useBusyFiles(t *testing.T, sync func(path string, mode fs.FileMode) (uint64, error), delay time.Duration) {
	t.Helper()
	prevSync, prevBusy, prevDelay := syncExtracted, fileBusy, verifyRetryDelay
	t.Cleanup(func() { syncExtracted, fileBusy, verifyRetryDelay = prevSync, prevBusy, prevDelay })
	syncExtracted = sync
	fileBusy = func(err error) bool { return errors.Is(err, errBusyFile) }
	verifyRetryDelay = delay
}

func TestVerificationSyncRetries(t *testing.T) {
	errOther := errors.New("i/o error")
	cases := []struct {
		name      string
		failures  int
		failWith  error
		wantErr   error
		wantCalls int
	}{
		{name: "no failure", wantCalls: 1},
		{name: "busy twice, then fine", failures: 2, failWith: errBusyFile, wantCalls: 3},
		{name: "busy until the retries run out", failures: 1000, failWith: errBusyFile, wantErr: errBusyFile, wantCalls: verifyRetries + 1},
		{name: "other errors are not retried", failures: 1000, failWith: errOther, wantErr: errOther, wantCalls: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			useBusyFiles(t, func(string, fs.FileMode) (uint64, error) {
				calls++
				if calls <= tc.failures {
					return 0, tc.failWith
				}
				return 1, nil
			}, time.Millisecond)
			v := &verification{ctx: context.Background(), budget: time.Hour}
			links, err := v.sync("file", 0o600)
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && links != 1) {
				t.Fatalf("links = %d, err = %v, want %v", links, err, tc.wantErr)
			}
			if calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}

func TestVerificationSyncWaitsAtMostTheBudgetForTheWholeExtraction(t *testing.T) {
	calls := map[string]int{}
	useBusyFiles(t, func(path string, _ fs.FileMode) (uint64, error) {
		calls[path]++
		if calls[path] <= 2 {
			return 0, errBusyFile
		}
		return 1, nil
	}, time.Millisecond)
	v := &verification{ctx: context.Background(), budget: 3 * time.Millisecond}
	if _, err := v.sync("a", 0o600); err != nil {
		t.Fatalf("first file: %v", err)
	}
	if _, err := v.sync("b", 0o600); !errors.Is(err, errBusyFile) {
		t.Fatalf("second file: err = %v, want the busy error once the shared budget is spent", err)
	}
	if calls["a"] != 3 || calls["b"] != 2 {
		t.Fatalf("calls = %v, want 3 for the first file and 2 for the second (one wait, then no budget)", calls)
	}
	if v.budget >= verifyRetryDelay {
		t.Fatalf("budget left = %v, want less than one delay", v.budget)
	}
}

func TestExtractionStopsWhenFilesStayBusyPastTheBudget(t *testing.T) {
	calls := map[string]int{}
	useBusyFiles(t, func(path string, _ fs.FileMode) (uint64, error) {
		calls[path]++
		if calls[path] <= 2 {
			return 0, errBusyFile
		}
		return 1, nil
	}, time.Millisecond)
	prevBudget := verifyRetryBudget
	t.Cleanup(func() { verifyRetryBudget = prevBudget })
	archive := storedRar(t, brokenGame(), laterEntry("Game/later1.bin"), laterEntry("Game/later2.bin"))
	plan := fakePlan{Files: map[string]string{"Game/game.exe": toolPayload, "Game/later1.bin": laterContent, "Game/later2.bin": laterContent}}

	verifyRetryBudget = time.Hour
	useTools(t, fakePlanTool(t, plan))
	if err := ExtractArchive(context.Background(), archive, filepath.Join(t.TempDir(), "out"), nil); err != nil {
		t.Fatalf("with a generous budget: %v", err)
	}

	clear(calls)
	verifyRetryBudget = 3 * time.Millisecond
	err := ExtractArchive(context.Background(), archive, filepath.Join(t.TempDir(), "out"), nil)
	if !errors.Is(err, errArchiveVerify) || !errors.Is(err, errBusyFile) {
		t.Fatalf("with 3 ms of budget for 6 waits: err = %v, want errArchiveVerify with the busy cause", err)
	}
}

func TestVerificationSyncStopsWaitingOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	useBusyFiles(t, func(string, fs.FileMode) (uint64, error) {
		calls++
		cancel()
		return 0, errBusyFile
	}, time.Hour)
	v := &verification{ctx: ctx, budget: 2 * time.Hour}
	done := make(chan error, 1)
	go func() {
		_, err := v.sync("file", 0o600)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatalf("err = %v after %d calls, want context.Canceled right after the first", err, calls)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("sync kept waiting after the context was cancelled")
	}
}

func TestToolRejectingTheSafetySwitchIsRefusedNotRerun(t *testing.T) {
	cases := []struct {
		name string
		make func(path string) archiveTool
		want string
	}{
		{name: "7-Zip", make: sevenZipTool, want: "-snl-"},
		{name: "UnRAR", make: fakeUnrar, want: "-ol-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rejecting := fakeAs(t, tc.make, fakePlan{Stdout: "Command Line Error:\nIncorrect switch postfix:\n", Exit: 7})
			var calls [][]string
			inner := rejecting.args
			rejecting.args = func(archive, dest string) []string {
				args := inner(archive, dest)
				calls = append(calls, args)
				return args
			}
			useTools(t, rejecting, fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}}))
			dest := filepath.Join(t.TempDir(), "out")
			if err := ExtractArchive(context.Background(), brokenRar(t), dest, nil); err != nil {
				t.Fatalf("ExtractArchive: %v", err)
			}
			if len(calls) != 1 {
				t.Fatalf("the rejecting tool ran %d times, want once and never without the switch", len(calls))
			}
		})
	}
	t.Run("nothing else to try", func(t *testing.T) {
		useTools(t, fakeAs(t, sevenZipTool, fakePlan{Stdout: "Command Line Error:\nIncorrect switch postfix:\n", Exit: 7}))
		err := ExtractArchive(context.Background(), brokenRar(t), filepath.Join(t.TempDir(), "out"), nil)
		if !errors.Is(err, errToolOutdated) || !errors.Is(err, errArchiveToolFailed) {
			t.Fatalf("err = %v, want errArchiveToolFailed that names an outdated tool", err)
		}
	})
}

func TestUnrarRenamingAnInvalidNameIsANameProblemNotADiskProblem(t *testing.T) {
	out := "Cannot create C:\\out\\Game\\q?x.txt\nWARNING: Attempting to correct the invalid file or directory name\nRenaming C:\\out\\Game\\q?x.txt to C:\\out\\Game\\q_x.txt\n"
	chainTools(t, fakeAs(t, fakeUnrar, fakePlan{Stdout: out, Exit: 9}))
	dest := filepath.Join(t.TempDir(), "out")
	err := ExtractArchive(context.Background(), brokenRar(t), dest, nil)
	if !errors.Is(err, errArchiveMismatch) || !errors.Is(err, errArchiveUnlisted) || errors.Is(err, errToolWrite) {
		t.Fatalf("err = %v, want a name mismatch and not a disk write failure", err)
	}
	if secondToolRan(t, dest) {
		t.Fatal("the second tool ran although every tool renames the same name")
	}
}

func TestUnrarExitCodesThatMeanDamageStopTheChain(t *testing.T) {
	cases := []struct {
		name   string
		plan   fakePlan
		wantIs []error
		stops  bool
	}{
		{name: "exit 3: invalid checksum, data is damaged", plan: fakePlan{Stdout: "Game\\f.bin - checksum error\n", Exit: 3}, wantIs: []error{errArchiveCorrupt}, stops: true},
		{name: "exit 13: bad archive", plan: fakePlan{Stdout: "Bad archive\n", Exit: 13}, wantIs: []error{errArchiveCorrupt}, stops: true},
		{name: "exit 3 with the truncation phrase is a truncation", plan: fakePlan{Stdout: "Unexpected end of archive\nGame\\f.bin - checksum error\n", Exit: 3}, wantIs: []error{errArchiveIncomplete}, stops: true},
		{name: "exit 12: read error is not a verdict on the archive", plan: fakePlan{Stdout: "Read error\n", Exit: 12}, wantIs: []error{errToolExit}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chainTools(t, fakeAs(t, fakeUnrar, tc.plan))
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), brokenRar(t), dest, nil)
			if tc.stops {
				for _, want := range tc.wantIs {
					if !errors.Is(err, want) {
						t.Fatalf("err = %v, want errors.Is %v", err, want)
					}
				}
				if errors.Is(err, errArchiveToolFailed) || secondToolRan(t, dest) {
					t.Fatalf("err = %v, second tool ran = %v: the chain must stop", err, secondToolRan(t, dest))
				}
				return
			}
			if !errors.Is(err, errToolExit) || !secondToolRan(t, dest) {
				t.Fatalf("err = %v, second tool ran = %v, want the chain to go on to the second tool", err, secondToolRan(t, dest))
			}
		})
	}
}

func TestToolPhrasesSurviveOutputLongerThanTheTail(t *testing.T) {
	noise := strings.Repeat("Extracting  Game\\f.bin  OK\n", 400)
	cases := []struct {
		name string
		tool func(t *testing.T) archiveTool
		want error
	}{
		{
			name: "7-Zip, phrase first on stderr",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, sevenZipTool, fakePlan{Stderr: "ERRORS:\nUnexpected end of archive\n" + noise, Exit: 2})
			},
			want: errArchiveIncomplete,
		},
		{
			name: "7-Zip, write failure first on stderr",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, sevenZipTool, fakePlan{Stderr: "ERROR: Cannot open output file : Access is denied. : x\n" + noise, Exit: 2})
			},
			want: errToolWrite,
		},
		{
			name: "UnRAR, phrase early on stdout",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, fakeUnrar, fakePlan{Stdout: "Unexpected end of archive\n" + noise, Exit: 3})
			},
			want: errArchiveIncomplete,
		},
		{
			name: "UnRAR, rename warning early on stdout",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, fakeUnrar, fakePlan{Stdout: "WARNING: Attempting to correct the invalid file or directory name\n" + noise, Exit: 9})
			},
			want: errArchiveUnlisted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chainTools(t, tc.tool(t))
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), brokenRar(t), dest, nil)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want errors.Is %v although the phrase is far outside the last kilobyte", err, tc.want)
			}
			if secondToolRan(t, dest) {
				t.Fatal("the second tool ran")
			}
		})
	}
}

func TestMarkerScanFindsPhrasesAcrossChunks(t *testing.T) {
	markers := []string{"unexpected end of archive"}
	text := "ERRORS:\nUnexpected end of archive\n"
	for split := 0; split <= len(text); split++ {
		scan := newMarkerScan(markers)
		scan.feed(text[:split])
		scan.feed(text[split:])
		if !scan.found(markers) {
			t.Fatalf("phrase lost when the stream is cut at byte %d", split)
		}
	}
	scan := newMarkerScan(markers)
	for _, chunk := range []string{"unexpected ", "end ", "of ", "archive"} {
		scan.feed(chunk)
	}
	if !scan.found(markers) {
		t.Fatal("phrase lost when it arrives word by word")
	}
	other := newMarkerScan(markers)
	other.feed("unexpected end of the line\narchive")
	if other.found(markers) {
		t.Fatal("a phrase assembled from unrelated words was found")
	}
	var none *markerScan
	none.feed("anything")
	if none.found(markers) {
		t.Fatal("a nil scan must find nothing")
	}
}

func TestToolReportingATruncatedArchiveStopsTheChain(t *testing.T) {
	cases := []struct {
		name string
		tool func(t *testing.T) archiveTool
	}{
		{
			name: "7-Zip",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, sevenZipTool, fakePlan{Stderr: "ERRORS:\nUnexpected end of archive\n\nERROR: Data Error : Game\\f2.bin\n", Exit: 2})
			},
		},
		{
			name: "UnRAR",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, fakeUnrar, fakePlan{Stdout: "Extracting  Game\\f1.bin  60%  OK\nUnexpected end of archive\nGame\\f2.bin - checksum error\nTotal errors: 3\n", Exit: 3})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chainTools(t, tc.tool(t))
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), brokenRar(t), dest, nil)
			if !errors.Is(err, errArchiveIncomplete) || errors.Is(err, errArchiveToolFailed) {
				t.Fatalf("err = %v, want errArchiveIncomplete itself", err)
			}
			if secondToolRan(t, dest) {
				t.Fatal("the second tool ran after the first one reported a truncated archive")
			}
		})
	}
}
