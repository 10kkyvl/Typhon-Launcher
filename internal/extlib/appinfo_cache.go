package extlib

import (
	"context"
	"os"
	"sync"
	"time"
)

type appTypeCache struct {
	mu      sync.Mutex
	path    string
	size    int64
	mod     time.Time
	covered map[uint32]bool
	types   map[string]string
	read    func(ctx context.Context, path string, wanted map[uint32]bool) (map[string]string, error)
}

var steamTypeCache = &appTypeCache{read: readAppTypes}

// Синхронизация идёт каждые пять минут, а appinfo.vdf в сотни мегабайт
// меняется редко: перечитываем только по смене размера, mtime или набора appid.
func (c *appTypeCache) get(ctx context.Context, path string, wanted map[uint32]bool) (map[string]string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	same := c.path == path && c.size == st.Size() && c.mod.Equal(st.ModTime())
	if same {
		missing := false
		for id := range wanted {
			if !c.covered[id] {
				missing = true
				break
			}
		}
		if !missing {
			return c.types, nil
		}
	}
	query := make(map[uint32]bool, len(wanted)+len(c.covered))
	for id := range wanted {
		query[id] = true
	}
	if same {
		for id := range c.covered {
			query[id] = true
		}
	}
	types, err := c.read(ctx, path, query)
	if err != nil {
		return nil, err
	}
	c.path, c.size, c.mod, c.covered, c.types = path, st.Size(), st.ModTime(), query, types
	return types, nil
}
