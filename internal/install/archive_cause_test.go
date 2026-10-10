package install

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	rardecode "github.com/nwaples/rardecode/v2"

	"typhon/internal/usagestats"
)

func TestShortFileCauseSurvivesExternalToolFailure(t *testing.T) {
	cases := []struct {
		name      string
		modes     []string
		writeFail bool
		wantIs    []error
		wantText  []string
		wantCode  string
	}{
		{
			name:     "tool fails",
			modes:    []string{"fail"},
			wantIs:   []error{errArchiveToolFailed, errToolExit, rardecode.ErrShortFile},
			wantText: []string{"decoded file too short", "код 2"},
			wantCode: "install_archive_tool_failed",
		},
		{
			name:     "no tool installed",
			wantIs:   []error{errArchiveToolMissing, rardecode.ErrShortFile},
			wantText: []string{"decoded file too short"},
			wantCode: "install_archive_tool_missing",
		},
		{
			name:      "tool cannot write",
			modes:     []string{"writefail"},
			writeFail: true,
			wantIs:    []error{errToolWrite, rardecode.ErrShortFile},
			wantText:  []string{"decoded file too short", "Write error"},
			wantCode:  "install_archive_tool_write_failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tools []archiveTool
			for _, m := range tc.modes {
				tool := fakeTool(t, m)
				if tc.writeFail {
					tool.exitCauses = map[int]error{5: errToolWrite}
				}
				tools = append(tools, tool)
			}
			useTools(t, tools...)
			err := ExtractArchive(context.Background(), storedRar(t, shortGame()), filepath.Join(t.TempDir(), "out"), nil)
			if err == nil {
				t.Fatal("ExtractArchive succeeded")
			}
			for _, want := range tc.wantIs {
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want errors.Is %v", err, want)
				}
			}
			for _, want := range tc.wantText {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("err = %v, want it to mention %q", err, want)
				}
			}
			if got := usagestats.Classify(err); got != tc.wantCode {
				t.Fatalf("Classify = %q, want %q for %v", got, tc.wantCode, err)
			}
		})
	}
}
