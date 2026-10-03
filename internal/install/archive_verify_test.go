package install

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var laterContent = strings.Repeat("t", 40)

func laterEntry(name string) rarEntry {
	return rarEntry{name: name, data: []byte(laterContent)}
}

func TestExtractRarVerifiesToolResult(t *testing.T) {
	linkTarget := t.TempDir()
	cases := []struct {
		name    string
		before  []rarEntry
		after   []rarEntry
		tool    func(t *testing.T) archiveTool
		wantIs  []error
		wantNot []error
		wantIn  []string
	}{
		{
			name: "result matches the listing",
			tool: func(t *testing.T) archiveTool {
				return fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}})
			},
		},
		{
			name: "file that is not in the listing",
			tool: func(t *testing.T) archiveTool {
				return fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload, "Game/extra.dll": "x"}})
			},
			wantIs:  []error{errArchiveMismatch, errArchiveUnlisted},
			wantNot: []error{errArchiveUnsafe, errArchiveToolFailed},
			wantIn:  []string{"extra.dll"},
		},
		{
			name:  "name the file system changed",
			after: []rarEntry{laterEntry("Game/what?.bin")},
			tool: func(t *testing.T) archiveTool {
				return fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload, "Game/what_.bin": laterContent}})
			},
			wantIs:  []error{errArchiveMismatch, errArchiveUnlisted},
			wantNot: []error{errArchiveUnsafe, errArchiveToolFailed},
			wantIn:  []string{"what_.bin"},
		},
		{
			name:   "regular file with a second hard link",
			before: []rarEntry{{name: "Game/copy.exe", data: []byte(toolPayload)}},
			tool: func(t *testing.T) archiveTool {
				return fakePlanTool(t, fakePlan{
					Files:     map[string]string{"Game/game.exe": toolPayload},
					HardLinks: map[string]string{"Game/copy.exe": "Game/game.exe"},
				})
			},
			wantIs: []error{errArchiveUnsafe, errArchiveHardLinked},
			wantIn: []string{"copy.exe"},
		},
		{
			name:   "listed directory replaced by a link",
			before: []rarEntry{{name: "Game/lnk", dir: true}},
			tool: func(t *testing.T) archiveTool {
				return fakePlanTool(t, fakePlan{
					Files:    map[string]string{"Game/game.exe": toolPayload},
					DirLinks: map[string]string{"Game/lnk": linkTarget},
				})
			},
			wantIs: []error{errArchiveUnsafe, errArchiveHasLinks},
			wantIn: []string{"lnk"},
		},
		{
			name: "file shorter than the listing",
			tool: func(t *testing.T) archiveTool {
				return fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": "short"}})
			},
			wantIs:  []error{errArchiveToolFailed, errToolOutput},
			wantNot: []error{errArchiveUnsafe},
			wantIn:  []string{"Game/game.exe", "ожидалось 9 байт, на диске 5"},
		},
		{
			name:  "listed files the tool never wrote",
			after: []rarEntry{laterEntry("Game/later1.bin"), laterEntry("Game/later2.bin")},
			tool: func(t *testing.T) archiveTool {
				return fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}})
			},
			wantIs:  []error{errArchiveMismatch, errFileGone},
			wantNot: []error{errArchiveUnsafe, errArchiveToolFailed},
			wantIn:  []string{"Game/later1.bin", "антивирус"},
		},
		{
			name:  "7-Zip warning with a complete result",
			after: []rarEntry{laterEntry("Game/later1.bin")},
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, sevenZipTool, fakePlan{
					Files: map[string]string{"Game/game.exe": toolPayload, "Game/later1.bin": laterContent},
					Exit:  1,
				})
			},
		},
		{
			name:  "7-Zip warning with a missing file",
			after: []rarEntry{laterEntry("Game/later1.bin")},
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, sevenZipTool, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}, Exit: 1})
			},
			wantIs: []error{errArchiveMismatch, errFileGone},
			wantIn: []string{"Game/later1.bin"},
		},
		{
			name: "UnRAR warning stays a failure",
			tool: func(t *testing.T) archiveTool {
				return fakeAs(t, fakeUnrar, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload}, Exit: 1})
			},
			wantIs: []error{errArchiveToolFailed, errToolExit},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useTools(t, tc.tool(t))
			entries := append(slices.Clone(tc.before), brokenGame())
			archive := storedRar(t, append(entries, tc.after...)...)
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), archive, dest, nil)
			if len(tc.wantIs) == 0 {
				if err != nil {
					t.Fatalf("ExtractArchive: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("ExtractArchive accepted a result that does not match the archive listing")
			}
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
			for _, want := range tc.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("err = %v, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestExtractRarSyncsEveryToolFile(t *testing.T) {
	var synced []string
	real := syncExtracted
	syncExtracted = func(path string, mode fs.FileMode) (uint64, error) {
		synced = append(synced, filepath.Base(path))
		return real(path, mode)
	}
	t.Cleanup(func() { syncExtracted = real })
	useTools(t, fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload, "Game/later1.bin": laterContent}}))
	archive := storedRar(t, brokenGame(), laterEntry("Game/later1.bin"))
	if err := ExtractArchive(context.Background(), archive, filepath.Join(t.TempDir(), "out"), nil); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	slices.Sort(synced)
	if want := []string{"game.exe", "later1.bin"}; !slices.Equal(synced, want) {
		t.Fatalf("synced = %v, want %v: tool-written files must reach the disk like the builtin ones", synced, want)
	}
}
