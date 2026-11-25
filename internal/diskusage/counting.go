package diskusage

import (
	"path/filepath"
	"strings"

	"typhon/internal/platform"
)

// countedAt is where a row's measured bytes lie on disk, for the totals to
// count each byte once; empty when they lie behind a link, where no walk of a
// folder around the link counted them.
func countedAt(path string, m measured) string {
	if m.rootLink || m.missing {
		return ""
	}
	return path
}

// Row ranks: of two rows at the same folder, the lower rank holds the bytes.
const (
	rankGame = iota
	rankLeftover
	rankDownload
)

type countedRow struct {
	path string
	rank int
	size int64
}

// ownBytes is, for every row, its size less the rows directly within it: the
// bytes the totals count for it. A row's own size stays what its folder
// holds; only the sums drop what an inner row already counts. A game installed
// inside its download's folder is counted as the game, and the download adds
// only what lies around it. Each row is taken off only the innermost row
// around it, which already holds the rest of the chain.
func ownBytes(rows []countedRow) []int64 {
	keys := make([]string, len(rows))
	for i, r := range rows {
		if r.path == "" {
			continue
		}
		if key, err := platform.PathKey(r.path); err == nil {
			keys[i] = key
		}
	}
	// within says row a's bytes are part of row b's measured size.
	within := func(a, b int) bool {
		if a == b || keys[a] == "" || keys[b] == "" {
			return false
		}
		if keys[a] == keys[b] {
			return rows[a].rank < rows[b].rank
		}
		prefix := keys[b]
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		return strings.HasPrefix(keys[a], prefix)
	}
	// inner says b lies within c: of two rows around the same row, the inner
	// one is the deeper folder, or the closer rank at the same folder.
	inner := func(b, c int) bool {
		if len(keys[b]) != len(keys[c]) {
			return len(keys[b]) > len(keys[c])
		}
		return rows[b].rank < rows[c].rank
	}
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.size
	}
	for a := range rows {
		parent := -1
		for b := range rows {
			if within(a, b) && (parent < 0 || inner(b, parent)) {
				parent = b
			}
		}
		if parent >= 0 {
			out[parent] -= rows[a].size
		}
	}
	for i := range out {
		out[i] = max(out[i], 0)
	}
	return out
}
