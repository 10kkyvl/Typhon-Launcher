package install

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	highConfidenceScore = 60
	highConfidenceGap   = 20
	maxCandidates       = 12
)

var excludedPrefixes = []string{
	"setup",
	"install",
	"update",
	"cleanup",
	"repair",
	"config",
}

var excludedParts = []string{
	"unins",
	"uninstall",
	"crashreport",
	"crashhandler",
	"crashsender",
	"unitycrash",
	"redist",
	"prereq",
	"dxsetup",
	"dxwebsetup",
	"oalinst",
	"directx",
	"dotnet",
	"vcredist",
	"vc_redist",
	"activation",
	"helper",
	"dwtrig",
}

var binDirs = map[string]bool{
	"bin":      true,
	"binaries": true,
	"win64":    true,
	"win32":    true,
	"x64":      true,
	"x86":      true,
	"game":     true,
}

var assetExts = map[string]bool{
	".dll":    true,
	".pak":    true,
	".dat":    true,
	".bank":   true,
	".assets": true,
	".arc":    true,
	".bnk":    true,
	".pck":    true,
	".vpk":    true,
	".rpf":    true,
	".big":    true,
	".ress":   true,
}

var assetDirs = map[string]bool{
	"data":    true,
	"assets":  true,
	"content": true,
	"bin":     true,
	"engine":  true,
	"game":    true,
	"res":     true,
	"media":   true,
	"sound":   true,
	"audio":   true,
	"movies":  true,
	"locale":  true,
}

func FindExecutables(ctx context.Context, root, title string) ([]Candidate, error) {
	wanted := normalizeName(title)
	var out []Candidate
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		base := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if excludedExe(base) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path %s: %w", path, err)
		}
		out = append(out, Candidate{Path: path, Score: scoreExe(rel, base, wanted, info.Size())})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("scan executables %s: %w", root, walkErr)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Path < out[j].Path
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out, nil
}

func HighConfidence(c []Candidate) bool {
	if len(c) == 0 {
		return false
	}
	if c[0].Score < highConfidenceScore {
		return false
	}
	if len(c) == 1 {
		return true
	}
	return c[0].Score-c[1].Score >= highConfidenceGap
}

func scoreExe(rel, base, wanted string, size int64) float64 {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	depth := len(parts) - 1

	score := math.Max(0, 40-float64(depth)*14)
	for _, part := range parts[:depth] {
		if binDirs[strings.ToLower(part)] {
			score += 18
			break
		}
	}
	score += similarity(normalizeName(base), wanted)
	if size > 0 {
		score += math.Min(30, math.Log10(float64(size)/1024+1)*8)
	}
	return math.Round(score*100) / 100
}

func similarity(name, wanted string) float64 {
	if name == "" || wanted == "" {
		return 0
	}
	if name == wanted {
		return 45
	}
	if strings.Contains(wanted, name) || strings.Contains(name, wanted) {
		return 30
	}
	shared := commonPrefix(name, wanted)
	if shared >= 4 {
		return math.Min(20, float64(shared)*3)
	}
	return 0
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func normalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 'а' && r <= 'я') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func excludedExe(base string) bool {
	name := strings.ToLower(base)
	flat := normalizeName(base)
	for _, p := range excludedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, p := range excludedParts {
		if strings.Contains(name, p) || strings.Contains(flat, p) {
			return true
		}
	}
	return false
}

func hasAssets(root string) (bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, fmt.Errorf("read dir %s: %w", root, err)
	}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if e.IsDir() {
			if assetDirs[name] || strings.HasSuffix(name, "_data") || strings.HasSuffix(name, "content") {
				return true, nil
			}
			continue
		}
		if assetExts[strings.ToLower(filepath.Ext(name))] {
			return true, nil
		}
	}
	return false, nil
}
