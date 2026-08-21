package updates

import (
	"container/heap"
	"strings"

	"typhon/internal/version"
)

type PatchPath struct {
	Steps   []Patch `json:"steps"`
	Bytes   int64   `json:"bytes"`
	Unknown int     `json:"unknown"`
}

type pathCost struct {
	unknown int
	bytes   int64
	steps   int
}

func (c pathCost) add(p Patch) pathCost {
	next := pathCost{unknown: c.unknown, bytes: c.bytes, steps: c.steps + 1}
	if p.Size > 0 {
		next.bytes += p.Size
	} else {
		next.unknown++
	}
	return next
}

func (c pathCost) less(other pathCost) bool {
	if c.unknown != other.unknown {
		return c.unknown < other.unknown
	}
	if c.bytes != other.bytes {
		return c.bytes < other.bytes
	}
	return c.steps < other.steps
}

func VersionKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if key := version.Key(version.Parse(trimmed)); key != "" {
		return key
	}
	return strings.ToLower(trimmed)
}

type edge struct {
	to    string
	patch Patch
}

type node struct {
	key   string
	cost  pathCost
	index int
}

type queue []*node

func (q queue) Len() int      { return len(q) }
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }

func (q queue) Less(i, j int) bool {
	if q[i].cost.less(q[j].cost) {
		return true
	}
	if q[j].cost.less(q[i].cost) {
		return false
	}
	return q[i].key < q[j].key
}

func (q *queue) Push(x any) {
	n := x.(*node)
	n.index = len(*q)
	*q = append(*q, n)
}

func (q *queue) Pop() any {
	old := *q
	n := old[len(old)-1]
	*q = old[:len(old)-1]
	return n
}

// FindPatchPath returns the cheapest chain of patches leading from one version to
// another. Cost is measured in download bytes, patches of unknown size are avoided.
func FindPatchPath(patches []Patch, from, to string) (PatchPath, bool) {
	start, target := VersionKey(from), VersionKey(to)
	if start == "" || target == "" || start == target {
		return PatchPath{}, false
	}

	graph := map[string][]edge{}
	for _, p := range patches {
		fromKey, toKey := VersionKey(p.FromVersion), VersionKey(p.ToVersion)
		if fromKey == "" || toKey == "" || fromKey == toKey {
			continue
		}
		graph[fromKey] = append(graph[fromKey], edge{to: toKey, patch: p})
	}
	for key := range graph {
		sortEdges(graph[key])
	}
	if len(graph[start]) == 0 {
		return PatchPath{}, false
	}

	best := map[string]pathCost{start: {}}
	visited := map[string]bool{}
	previous := map[string]edge{}

	pending := &queue{}
	heap.Push(pending, &node{key: start})
	for pending.Len() > 0 {
		current := heap.Pop(pending).(*node)
		if visited[current.key] {
			continue
		}
		visited[current.key] = true
		if current.key == target {
			break
		}
		for _, e := range graph[current.key] {
			if visited[e.to] {
				continue
			}
			next := current.cost.add(e.patch)
			if known, ok := best[e.to]; ok && !next.less(known) {
				continue
			}
			best[e.to] = next
			previous[e.to] = edge{to: current.key, patch: e.patch}
			heap.Push(pending, &node{key: e.to, cost: next})
		}
	}

	if !visited[target] {
		return PatchPath{}, false
	}

	var steps []Patch
	for key := target; key != start; {
		prev, ok := previous[key]
		if !ok {
			return PatchPath{}, false
		}
		steps = append(steps, prev.patch)
		key = prev.to
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}

	cost := best[target]
	return PatchPath{Steps: steps, Bytes: cost.bytes, Unknown: cost.unknown}, true
}

func sortEdges(edges []edge) {
	for i := 1; i < len(edges); i++ {
		for j := i; j > 0 && edgeLess(edges[j], edges[j-1]); j-- {
			edges[j], edges[j-1] = edges[j-1], edges[j]
		}
	}
}

func edgeLess(a, b edge) bool {
	if a.patch.Priority != b.patch.Priority {
		return a.patch.Priority > b.patch.Priority
	}
	if a.patch.Size != b.patch.Size {
		return a.patch.Size < b.patch.Size
	}
	return a.patch.ID < b.patch.ID
}
