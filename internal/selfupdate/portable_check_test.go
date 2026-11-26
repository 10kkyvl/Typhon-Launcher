package selfupdate

import (
	"context"
	"errors"
	"testing"
)

// Release tooling asks CheckPortableZip whether a zip it made would be
// applied; the answer must be the one the portable swap itself gives.
func TestCheckPortableZipJudgesAsTheSwapDoes(t *testing.T) {
	cases := map[string]struct {
		entries []portableZipEntry
		want    error
	}{
		"one executable": {[]portableZipEntry{{"typhon.exe", newExe}}, nil},
		"empty archive":  {nil, ErrPortableZipNoExecutable},
		"nested only":    {[]portableZipEntry{{"app/typhon.exe", newExe}}, ErrPortableZipNoExecutable},
		"duplicate":      {[]portableZipEntry{{"typhon.exe", newExe}, {"typhon.exe", "other"}}, ErrPortableZipExecutable},
		"empty binary":   {[]portableZipEntry{{"typhon.exe", ""}}, ErrPortableZipExecutable},
		"unsafe entry":   {[]portableZipEntry{{"typhon.exe", newExe}, {"../x", "x"}}, ErrPortableZipUnsafeEntry},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			zipPath := buildZip(t, tc.entries...)
			checkErr := CheckPortableZip(zipPath)
			applyErr := applyPortableZip(context.Background(), zipPath, portableTarget(t))
			if tc.want == nil {
				if checkErr != nil || applyErr != nil {
					t.Fatalf("CheckPortableZip = %v, applyPortableZip = %v; want both to accept", checkErr, applyErr)
				}
				return
			}
			if !errors.Is(checkErr, tc.want) || !errors.Is(applyErr, tc.want) {
				t.Fatalf("CheckPortableZip = %v, applyPortableZip = %v; want both %v", checkErr, applyErr, tc.want)
			}
		})
	}
}

func TestCheckPortableZipRefusesAFileThatIsNoZip(t *testing.T) {
	if err := CheckPortableZip(t.TempDir()); err == nil {
		t.Fatal("CheckPortableZip accepted a directory")
	}
}
