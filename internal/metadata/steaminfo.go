package metadata

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"typhon/internal/storage"
)

const (
	steamInfoVersion  = 1
	steamInfoFileName = "steam_info.json"

	maxRequirementRows  = 20
	maxRequirementLabel = 64
	maxRequirementValue = 300
	maxDLC              = 50
	maxDLCName          = 200
	maxMetacritic       = 100

	ControllerFull    = "full"
	ControllerPartial = "partial"
)

var steamStoreURL = regexp.MustCompile(`^/app/([0-9]{1,18})/?$`)

type Requirement struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type SteamRequirements struct {
	Minimum     []Requirement `json:"minimum"`
	Recommended []Requirement `json:"recommended"`
}

type SteamDLC struct {
	AppID int64  `json:"appId"`
	Name  string `json:"name"`
}

// SteamInfo — данные магазина Steam о игре, которые каталог не хранит.
// Controller пуст, когда поддержка неизвестна; Metacritic и Recommendations
// nil, когда числа нет: ноль — настоящее значение, а не «нет данных».
type SteamInfo struct {
	AppID           int64             `json:"appId"`
	URL             string            `json:"url"`
	Requirements    SteamRequirements `json:"requirements"`
	DLC             []SteamDLC        `json:"dlc"`
	Controller      string            `json:"controller"`
	Metacritic      *int              `json:"metacritic"`
	Recommendations *int              `json:"recommendations"`
}

// SanitizeSteamInfo приводит блок от сервера к тому, что безопасно хранить и
// показывать. ok == false, когда блока нет или в нём нет корректного appId:
// без него ссылки и DLC привязать не к чему.
func SanitizeSteamInfo(in *SteamInfo) (*SteamInfo, bool) {
	if in == nil || in.AppID <= 0 {
		return nil, false
	}
	out := &SteamInfo{
		AppID: in.AppID,
		URL:   validSteamURL(in.URL, in.AppID),
		Requirements: SteamRequirements{
			Minimum:     cleanRequirements(in.Requirements.Minimum),
			Recommended: cleanRequirements(in.Requirements.Recommended),
		},
		DLC:             cleanDLC(in.DLC),
		Controller:      validController(in.Controller),
		Metacritic:      nonNegative(in.Metacritic, maxMetacritic),
		Recommendations: nonNegative(in.Recommendations, 0),
	}
	return out, true
}

func validSteamURL(raw string, appID int64) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != "store.steampowered.com" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return ""
	}
	m := steamStoreURL.FindStringSubmatch(u.Path)
	if m == nil || m[1] != strconv.FormatInt(appID, 10) {
		return ""
	}
	return "https://store.steampowered.com/app/" + m[1]
}

func SteamStoreURL(appID int64) string {
	return "https://store.steampowered.com/app/" + strconv.FormatInt(appID, 10)
}

func validController(v string) string {
	switch v {
	case ControllerFull, ControllerPartial:
		return v
	default:
		return ""
	}
}

func nonNegative(v *int, upper int) *int {
	if v == nil || *v < 0 || (upper > 0 && *v > upper) {
		return nil
	}
	n := *v
	return &n
}

func cleanText(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' || r == utf8.RuneError {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit]))
}

func cleanRequirements(in []Requirement) []Requirement {
	out := make([]Requirement, 0, min(len(in), maxRequirementRows))
	for _, row := range in {
		label := cleanText(row.Label, maxRequirementLabel)
		value := cleanText(row.Value, maxRequirementValue)
		if value == "" {
			continue
		}
		out = append(out, Requirement{Label: label, Value: value})
		if len(out) == maxRequirementRows {
			break
		}
	}
	return out
}

func cleanDLC(in []SteamDLC) []SteamDLC {
	out := make([]SteamDLC, 0, min(len(in), maxDLC))
	seen := map[int64]bool{}
	for _, d := range in {
		name := cleanText(d.Name, maxDLCName)
		if d.AppID <= 0 || name == "" || seen[d.AppID] {
			continue
		}
		seen[d.AppID] = true
		out = append(out, SteamDLC{AppID: d.AppID, Name: name})
		if len(out) == maxDLC {
			break
		}
	}
	return out
}

type steamEntry struct {
	GameID    string     `json:"gameId"`
	FetchedAt time.Time  `json:"fetchedAt"`
	Steam     *SteamInfo `json:"steam,omitempty"`
}

// steamStore помнит, у каких игр блок Steam уже запрашивали. Запись без блока
// — это «сервер ответил, что данных нет», и она отличается от отсутствия
// записи: без этого старый бэкенд гонял бы обновление при каждом открытии.
type steamStore struct {
	mu      sync.Mutex
	path    string
	entries map[string]steamEntry
	// introducedAt — когда файл создан впервые. Метаданные, сохранённые до
	// этого момента, не могли получить блок Steam, и такая игра обновляется
	// один раз, а не ждёт конца TTL.
	introducedAt time.Time
}

type steamFile struct {
	IntroducedAt time.Time    `json:"introducedAt"`
	Entries      []steamEntry `json:"entries"`
}

func newSteamStore(dir string, now time.Time) (*steamStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errStorePath
	}
	s := &steamStore{path: filepath.Join(dir, steamInfoFileName), entries: map[string]steamEntry{}}
	var stored steamFile
	err := storage.Load(s.path, steamInfoVersion, nil, &stored)
	if errors.Is(err, fs.ErrNotExist) {
		s.introducedAt = now
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.saveLocked(); err != nil {
			return nil, fmt.Errorf("create steam info: %w", err)
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load steam info: %w", err)
	}
	s.introducedAt = stored.IntroducedAt
	for _, e := range stored.Entries {
		if e.GameID != "" {
			s.entries[e.GameID] = e
		}
	}
	return s, nil
}

func (s *steamStore) get(gameID string) (steamEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[gameID]
	return e, ok
}

func (s *steamStore) predates(updatedAt *time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return updatedAt != nil && updatedAt.Before(s.introducedAt)
}

func (s *steamStore) info(gameID string) *SteamInfo {
	e, ok := s.get(gameID)
	if !ok {
		return nil
	}
	return e.Steam
}

func (s *steamStore) put(gameID string, info *SteamInfo, at time.Time) (restore func() error, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, had := s.entries[gameID]
	revert := func() {
		if had {
			s.entries[gameID] = prev
		} else {
			delete(s.entries, gameID)
		}
	}
	s.entries[gameID] = steamEntry{GameID: gameID, FetchedAt: at, Steam: info}
	if err := s.saveLocked(); err != nil {
		revert()
		return nil, fmt.Errorf("save steam info: %w", err)
	}
	return func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		revert()
		if err := s.saveLocked(); err != nil {
			return fmt.Errorf("restore steam info: %w", err)
		}
		return nil
	}, nil
}

func (s *steamStore) saveLocked() error {
	list := make([]steamEntry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e)
	}
	return storage.Save(s.path, steamInfoVersion, steamFile{IntroducedAt: s.introducedAt, Entries: list})
}
