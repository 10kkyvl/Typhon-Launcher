package diskusage

import "typhon/internal/platform"

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

// within says a's bytes are part of b's measured size.
func within(a, b countedRow) bool {
	if a.path == "" || b.path == "" {
		return false
	}
	if platform.SamePath(a.path, b.path) {
		return a.rank < b.rank
	}
	return platform.Inside(b.path, a.path)
}

// ownBytes is, for every row, its size less the rows directly within it: the
// bytes the totals count for it. A row's own size stays what its folder
// holds; only the sums drop what an inner row already counts. A game installed
// inside its download's folder is counted as the game, and the download adds
// only what lies around it.
func ownBytes(rows []countedRow) []int64 {
	out := make([]int64, len(rows))
	for i, outer := range rows {
		own := outer.size
		for j, inner := range rows {
			if i == j || !within(inner, outer) {
				continue
			}
			direct := true
			for k, mid := range rows {
				if k != i && k != j && within(inner, mid) && within(mid, outer) {
					direct = false
					break
				}
			}
			if direct {
				own -= inner.size
			}
		}
		out[i] = max(own, 0)
	}
	return out
}
