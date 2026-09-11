// Package catalogmigration prepares an offline, read-only report and commits
// only a redirect sidecar. Installation and distribution documents are never
// rewritten. The application must be stopped while applying a report.
package catalogmigration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"typhon/internal/catalog"
	"typhon/internal/storage"
)

type Group struct {
	Target   string   `json:"target"`
	IDs      []string `json:"ids"`
	Evidence string   `json:"evidence"`
}
type Report struct {
	Version          int                       `json:"version"`
	CreatedAt        time.Time                 `json:"createdAt"`
	Hashes           map[string]string         `json:"hashes"`
	Confident        []Group                   `json:"confident"`
	Review           [][]string                `json:"review"`
	WithoutProviders []string                  `json:"withoutProviders"`
	References       map[string]map[string]int `json:"references"`
	Redirects        map[string]string         `json:"redirects"`
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func Plan(dir string) (Report, error) {
	out := Report{Version: 1, CreatedAt: time.Now().UTC(), Hashes: map[string]string{}, References: map[string]map[string]int{}, Redirects: map[string]string{}}
	var games []catalog.Game
	if err := storage.Load(filepath.Join(dir, "catalog.json"), 1, nil, &games); err != nil {
		return out, err
	}
	if err := storage.Load(filepath.Join(dir, "catalog-redirects.json"), 1, nil, &out.Redirects); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return out, err
	}
	byKey := map[string][]catalog.Game{}
	byName := map[string][]string{}
	ids := map[string]bool{}
	parents := make([]int, len(games))
	claimsByRoot := make([]string, len(games))
	var root func(int) int
	root = func(i int) int {
		if parents[i] != i {
			parents[i] = root(parents[i])
		}
		return parents[i]
	}
	keys := map[string][]int{}
	for i, g := range games {
		if g.ID == "" || ids[g.ID] {
			return out, errors.New("invalid or duplicate catalog ID")
		}
		ids[g.ID] = true
		parents[i] = i
		claimsByRoot[i] = g.ExternalIDs.IGDB
		providerKeys := []string{}
		if g.ServerID != "" {
			providerKeys = append(providerKeys, "canonical:"+g.ServerID)
		}
		if g.ExternalIDs.IGDB != "" {
			providerKeys = append(providerKeys, "igdb:"+g.ExternalIDs.IGDB)
		}
		if g.ExternalIDs.Steam != "" {
			providerKeys = append(providerKeys, "steam:"+g.ExternalIDs.Steam)
		}
		for _, id := range g.ProviderLinks["steam"] {
			providerKeys = append(providerKeys, "steam:"+id)
		}
		if len(providerKeys) == 0 {
			out.WithoutProviders = append(out.WithoutProviders, g.ID)
		}
		for _, key := range providerKeys {
			keys[key] = append(keys[key], i)
		}
		name := strings.ToLower(strings.TrimSpace(g.Title))
		byName[name] = append(byName[name], g.ID)
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		members := keys[key]
		claims := map[string]bool{}
		for _, i := range members {
			if claim := claimsByRoot[root(i)]; claim != "" {
				claims[claim] = true
			}
		}
		if len(claims) > 1 {
			conflict := []string{}
			for _, i := range members {
				conflict = append(conflict, games[i].ID)
			}
			out.Review = append(out.Review, conflict)
			continue
		}
		for _, i := range members[1:] {
			parents[root(i)] = root(members[0])
		}
		for claim := range claims {
			claimsByRoot[root(members[0])] = claim
		}
	}
	for i, g := range games {
		key := fmt.Sprintf("provider component %d", root(i))
		byKey[key] = append(byKey[key], g)
	}
	for _, group := range byKey {
		if len(group) < 2 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			if !group[i].CreatedAt.Equal(group[j].CreatedAt) {
				return group[i].CreatedAt.Before(group[j].CreatedAt)
			}
			return group[i].ID < group[j].ID
		})
		target := group[0].ID
		delete(out.Redirects, target)
		proof := []string{}
		for _, game := range group {
			if game.ExternalIDs.IGDB != "" {
				proof = append(proof, "igdb:"+game.ExternalIDs.IGDB)
			}
			if game.ExternalIDs.Steam != "" {
				proof = append(proof, "steam:"+game.ExternalIDs.Steam)
			}
			if game.ServerID != "" {
				proof = append(proof, "canonical:"+game.ServerID)
			}
		}
		sort.Strings(proof)
		g := Group{Target: target, Evidence: strings.Join(proof, ", ")}
		for _, game := range group {
			g.IDs = append(g.IDs, game.ID)
			if game.ID != target {
				out.Redirects[game.ID] = target
			}
		}
		out.Confident = append(out.Confident, g)
	}
	for _, group := range byName {
		if len(group) > 1 {
			sort.Strings(group)
			out.Review = append(out.Review, group)
		}
	}
	for old := range out.Redirects {
		seen := map[string]bool{}
		for id := old; id != ""; id = out.Redirects[id] {
			if !ids[id] || seen[id] {
				return out, errors.New("invalid or cyclic existing redirects")
			}
			seen[id] = true
		}
	}
	sort.Slice(out.Review, func(i, j int) bool { return strings.Join(out.Review[i], "\x00") < strings.Join(out.Review[j], "\x00") })
	sort.Slice(out.Confident, func(i, j int) bool { return out.Confident[i].Target < out.Confident[j].Target })
	sort.Strings(out.WithoutProviders)
	scanRoot, err := os.OpenRoot(dir)
	if err != nil {
		return out, err
	}
	defer func() {
		if closeErr := scanRoot.Close(); closeErr != nil {
			slog.Warn("close catalog scan root", "error", closeErr)
		}
	}()
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "catalog-pages" || d.Name() == "catalog-migration-backups" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		rel, e := filepath.Rel(dir, path)
		if e != nil {
			return e
		}
		b, e := scanRoot.ReadFile(rel)
		if e != nil {
			return e
		}
		out.Hashes[rel] = digest(b)
		var data any
		if e = json.Unmarshal(b, &data); e != nil {
			return fmt.Errorf("%s: %w", rel, e)
		}
		counts := map[string]int{}
		var walk func(any, string)
		walk = func(v any, key string) {
			switch x := v.(type) {
			case map[string]any:
				for k, value := range x {
					walk(value, k)
				}
			case []any:
				for _, value := range x {
					walk(value, key)
				}
			case string:
				if ids[x] && key != "id" {
					counts[x]++
				}
			}
		}
		walk(data, "")
		if len(counts) > 0 {
			out.References[rel] = counts
		}
		return nil
	})
	return out, err
}

func Apply(dir string, report Report) error {
	if report.Version != 1 {
		return errors.New("unsupported report version")
	}
	for name, hash := range report.Hashes {
		if !filepath.IsLocal(name) {
			return errors.New("invalid report path")
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if digest(b) != hash {
			// A previous successful commit is safely repeatable.
			if name == "catalog-redirects.json" {
				var current map[string]string
				if storage.Load(filepath.Join(dir, name), 1, nil, &current) == nil {
					if reflect.DeepEqual(current, report.Redirects) {
						continue
					}
				}
			}
			return fmt.Errorf("data changed since report: %s", name)
		}
	}
	// Recompute the plan so a modified report cannot introduce name-only merges.
	fresh, err := Plan(dir)
	if err != nil {
		return err
	}
	for name := range fresh.Hashes {
		if _, ok := report.Hashes[name]; !ok && name != "catalog-redirects.json" {
			return fmt.Errorf("new data file since report: %s", name)
		}
	}
	for old, target := range report.Redirects {
		if fresh.Redirects[old] != target {
			return errors.New("redirect lacks current provider evidence")
		}
	}
	backup := filepath.Join(dir, "catalog-migration-backups", digest([]byte(fmt.Sprint(report.CreatedAt.UnixNano()))))
	if err = os.MkdirAll(backup, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "catalog-redirects.json")
	before, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		before = []byte(`{"version":1,"data":{}}`)
	} else if err != nil {
		return err
	}
	saved := filepath.Join(backup, "catalog-redirects.json")
	if existing, readErr := os.ReadFile(saved); errors.Is(readErr, fs.ErrNotExist) {
		if err = storage.WriteAtomic(saved, before); err != nil {
			return err
		}
	} else if readErr != nil {
		return readErr
	} else if !json.Valid(existing) {
		return errors.New("invalid redirect backup")
	}
	if err = storage.Save(filepath.Join(backup, "report.json"), 1, report); err != nil {
		return err
	}
	return storage.Save(path, 1, report.Redirects)
}

// Restore reverts the single redirect commit, retaining the original catalog
// and every personal document. Refuse to overwrite subsequent mapping edits.
func Restore(dir, backup string) error {
	var report Report
	if err := storage.Load(filepath.Join(backup, "report.json"), 1, nil, &report); err != nil {
		return err
	}
	var current map[string]string
	if err := storage.Load(filepath.Join(dir, "catalog-redirects.json"), 1, nil, &current); err != nil {
		return err
	}
	if !reflect.DeepEqual(current, report.Redirects) {
		return errors.New("redirects changed after migration")
	}
	var previous map[string]string
	if err := storage.Load(filepath.Join(backup, "catalog-redirects.json"), 1, nil, &previous); err != nil {
		return err
	}
	return storage.Save(filepath.Join(dir, "catalog-redirects.json"), 1, previous)
}
