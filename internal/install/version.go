package install

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const versionFileLimit = 64 << 10

type VersionInfo struct {
	Version    string `json:"version"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
	Product    string `json:"product"`
	Company    string `json:"company"`
}

var versionFileNames = map[string]bool{
	"version":      true,
	"version.txt":  true,
	"version.ini":  true,
	"version.dat":  true,
	"build.txt":    true,
	"gameversion":  true,
	"revision.txt": true,
}

var versionPattern = regexp.MustCompile(`\d+\.\d+(?:\.\d+)*`)

func VersionFromFiles(dir string) (VersionInfo, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return VersionInfo{}, false
	}
	for _, e := range entries {
		if e.IsDir() || !versionFileNames[strings.ToLower(e.Name())] {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > versionFileLimit {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		match := versionPattern.Find(data)
		if match == nil {
			continue
		}
		return VersionInfo{
			Version:    string(match),
			Source:     "version_file",
			Confidence: "medium",
		}, true
	}
	return VersionInfo{}, false
}
