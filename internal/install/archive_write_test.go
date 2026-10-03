package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useFreeSpace(t *testing.T, fn func(string, int64) error) {
	t.Helper()
	prev := archiveFreeSpace
	archiveFreeSpace = fn
	t.Cleanup(func() { archiveFreeSpace = prev })
}

func freeBytes(free int64) func(string, int64) error {
	return func(_ string, needed int64) error {
		if needed > free {
			return fmt.Errorf("%w: нужно %d байт, свободно %d", ErrNotEnoughSpace, needed, free)
		}
		return nil
	}
}

var (
	bigContent = strings.Repeat("b", 1000)
	errVolume  = errors.New("volume not found")
)

func unmeasurable(string, int64) error {
	return fmt.Errorf("%w: %w", ErrNotEnoughSpace, errVolume)
}

func bigRar(t *testing.T) string {
	t.Helper()
	return storedRar(t, brokenGame(), rarEntry{name: "Game/big.bin", data: []byte(bigContent)})
}

func TestSevenZipWriteErrorStopsTheChain(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
	}{
		{name: "access denied, russian system text", stderr: "ERROR: Cannot open output file : Отказано в доступе. : C:\\games\\Game\\game.exe\n"},
		{name: "access denied, english system text", stderr: "ERROR: Cannot open output file : Access is denied. : C:\\games\\Game\\game.exe\n"},
		{name: "file is locked", stderr: "ERROR: Cannot delete output file : The process cannot access the file because it is being used by another process. : C:\\games\\Game\\game.exe\n"},
		{name: "folder cannot be created", stderr: "ERROR: Cannot create folder : Cannot create a file when that file already exists. : C:\\games\\Game\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFreeSpace(t, freeBytes(1<<40))
			useTools(t,
				fakeAs(t, sevenZipTool, fakePlan{Stderr: tc.stderr, Exit: 2}),
				fakeTool(t, "ok"),
			)
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), brokenRar(t), dest, nil)
			if !errors.Is(err, errToolWrite) || errors.Is(err, errArchiveToolFailed) {
				t.Fatalf("err = %v, want the write failure itself, not a generic tool failure", err)
			}
			if !strings.Contains(err.Error(), "Cannot") {
				t.Fatalf("err = %v, want 7-Zip's own message", err)
			}
			data, readErr := os.ReadFile(filepath.Join(dest, "Game", "game.exe"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(data) == toolPayload {
				t.Fatal("the next tool ran after a disk write failure")
			}
		})
	}
}

func TestExtractRarToolFailureOnFullDisk(t *testing.T) {
	const diskFull = "System ERROR:\nНа диске недостаточно места.\n"
	cases := []struct {
		name       string
		firstFiles map[string]string
		withSecond bool
		free       func(string, int64) error
		success    bool
		wantIs     []error
		wantNot    []error
		wantIn     string
	}{
		{
			name:       "free space is below what is left to extract",
			withSecond: true,
			free:       freeBytes(100),
			wantIs:     []error{errNotEnoughSpace, errToolWrite, ErrNotEnoughSpace},
			wantNot:    []error{errArchiveToolFailed},
			wantIn:     "свободно 100",
		},
		{
			name:       "free space covers what is left",
			withSecond: true,
			free:       freeBytes(1 << 40),
			success:    true,
		},
		{
			name:       "free space is between what is left and the whole archive",
			withSecond: true,
			free:       freeBytes(1005),
			success:    true,
		},
		{
			name:       "a half-written file counts as written",
			withSecond: true,
			firstFiles: map[string]string{"Game/big.bin": strings.Repeat("b", 600)},
			free:       freeBytes(450),
			success:    true,
		},
		{
			name:       "a half-written file does not hide a full disk",
			withSecond: true,
			firstFiles: map[string]string{"Game/big.bin": strings.Repeat("b", 600)},
			free:       freeBytes(300),
			wantIs:     []error{errNotEnoughSpace, errToolWrite},
			wantNot:    []error{errArchiveToolFailed},
			wantIn:     "свободно 300",
		},
		{
			name:    "free space cannot be measured",
			free:    unmeasurable,
			wantIs:  []error{errArchiveToolFailed, errToolExit, errVolume},
			wantNot: []error{errNotEnoughSpace, errToolWrite},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFreeSpace(t, tc.free)
			tools := []archiveTool{fakeAs(t, sevenZipTool, fakePlan{Files: tc.firstFiles, Stderr: diskFull, Exit: 2})}
			if tc.withSecond {
				tools = append(tools, fakePlanTool(t, fakePlan{Files: map[string]string{"Game/game.exe": toolPayload, "Game/big.bin": bigContent}}))
			}
			useTools(t, tools...)
			dest := filepath.Join(t.TempDir(), "out")
			err := ExtractArchive(context.Background(), bigRar(t), dest, nil)
			data, readErr := os.ReadFile(filepath.Join(dest, "Game", "game.exe"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			secondRan := string(data) == toolPayload
			if tc.success {
				if err != nil || !secondRan {
					t.Fatalf("err = %v, second tool ran = %v, want the chain to go on to the second tool", err, secondRan)
				}
				return
			}
			if err == nil {
				t.Fatal("ExtractArchive succeeded")
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
			if tc.wantIn != "" && !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.wantIn)
			}
			if secondRan {
				t.Fatal("the second tool ran after a disk-full failure")
			}
		})
	}
}
