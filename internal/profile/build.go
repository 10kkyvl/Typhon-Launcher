package profile

import (
	"sort"
	"time"

	"typhon/internal/library"
	"typhon/internal/playlog"
)

const (
	recentWindow    = 14 * 24 * time.Hour
	maxPlaying      = 5
	maxActivityDays = 10
	maxShowcase     = 6
)

type Snapshot struct {
	Stats    Stats           `json:"stats"`
	Playing  []PlayingEntry  `json:"playing"`
	Activity []ActivityDay   `json:"activity"`
	Running  []GameRef       `json:"running"`
	Showcase []ShowcaseBlock `json:"showcase"`
}

type Stats struct {
	Games          int   `json:"games"`
	Hours          int   `json:"hours"`
	Completed      int   `json:"completed"`
	Playing        int   `json:"playing"`
	MonthSeconds   int64 `json:"monthSeconds"`
	MonthGames     int   `json:"monthGames"`
	MonthCompleted int   `json:"monthCompleted"`
}

type GameRef struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Cover           string     `json:"cover"`
	CanonicalGameID string     `json:"canonicalGameId,omitempty"`
	PlaytimeSeconds int64      `json:"playtimeSeconds"`
	Status          string     `json:"status"`
	StatusAt        *time.Time `json:"statusAt,omitempty"`
}

type PlayingEntry struct {
	Game          GameRef `json:"game"`
	RecentSeconds int64   `json:"recentSeconds"`
}

type ActivityDay struct {
	Date    string          `json:"date"`
	Entries []ActivityEntry `json:"entries"`
}

type ActivityEntry struct {
	Game    GameRef `json:"game"`
	Seconds int64   `json:"seconds"`
}

type ShowcaseBlock struct {
	Kind  string    `json:"kind"`
	Games []GameRef `json:"games"`
}

func Build(games []library.Game, sessions []playlog.Session, running []string, showcase []string, now time.Time) Snapshot {
	byID := make(map[string]library.Game, len(games))
	for _, g := range games {
		byID[g.ID] = g
	}

	snap := Snapshot{
		Playing:  []PlayingEntry{},
		Activity: []ActivityDay{},
		Running:  []GameRef{},
		Showcase: []ShowcaseBlock{},
	}

	var totalSeconds int64
	for _, g := range games {
		totalSeconds += g.PlaytimeSeconds
		if g.Status == library.StatusCompleted {
			snap.Stats.Completed++
		}
	}
	snap.Stats.Games = len(games)
	snap.Stats.Hours = int(totalSeconds / 3600)

	windowStart := now.Add(-recentWindow)
	recent := map[string]int64{}
	days := map[string]map[string]int64{}
	for _, s := range sessions {
		g, known := byID[s.GameID]
		if !known || !s.EndedAt.After(windowStart) {
			continue
		}
		start := s.StartedAt
		if start.Before(windowStart) {
			start = windowStart
		}
		seconds := int64(s.EndedAt.Sub(start).Seconds())
		if seconds <= 0 {
			continue
		}
		recent[g.ID] += seconds
		day := s.EndedAt.In(now.Location()).Format("2006-01-02")
		if days[day] == nil {
			days[day] = map[string]int64{}
		}
		days[day][g.ID] += seconds
	}

	snap.Stats.Playing = len(recent)
	for id, seconds := range recent {
		snap.Playing = append(snap.Playing, PlayingEntry{Game: ref(byID[id]), RecentSeconds: seconds})
	}
	sort.Slice(snap.Playing, func(i, j int) bool {
		if snap.Playing[i].RecentSeconds != snap.Playing[j].RecentSeconds {
			return snap.Playing[i].RecentSeconds > snap.Playing[j].RecentSeconds
		}
		return snap.Playing[i].Game.Title < snap.Playing[j].Game.Title
	})
	if len(snap.Playing) > maxPlaying {
		snap.Playing = snap.Playing[:maxPlaying]
	}

	dates := make([]string, 0, len(days))
	for day := range days {
		dates = append(dates, day)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	if len(dates) > maxActivityDays {
		dates = dates[:maxActivityDays]
	}
	for _, day := range dates {
		entries := make([]ActivityEntry, 0, len(days[day]))
		for id, seconds := range days[day] {
			entries = append(entries, ActivityEntry{Game: ref(byID[id]), Seconds: seconds})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Seconds != entries[j].Seconds {
				return entries[i].Seconds > entries[j].Seconds
			}
			return entries[i].Game.Title < entries[j].Game.Title
		})
		snap.Activity = append(snap.Activity, ActivityDay{Date: day, Entries: entries})
	}

	monthStart := MonthStart(now)
	monthGames := map[string]struct{}{}
	for _, s := range sessions {
		g, known := byID[s.GameID]
		if !known || s.EndedAt.Before(monthStart) {
			continue
		}
		start := s.StartedAt
		if start.Before(monthStart) {
			start = monthStart
		}
		seconds := int64(s.EndedAt.Sub(start).Seconds())
		if seconds <= 0 {
			continue
		}
		snap.Stats.MonthSeconds += seconds
		monthGames[g.ID] = struct{}{}
	}
	snap.Stats.MonthGames = len(monthGames)
	for _, g := range games {
		if g.Status == library.StatusCompleted && g.StatusAt != nil && !g.StatusAt.Before(monthStart) {
			snap.Stats.MonthCompleted++
		}
	}

	for _, id := range running {
		if g, ok := byID[id]; ok {
			snap.Running = append(snap.Running, ref(g))
		}
	}

	for _, kind := range showcase {
		snap.Showcase = append(snap.Showcase, ShowcaseBlock{Kind: kind, Games: showcaseGames(kind, games)})
	}
	return snap
}

func MonthStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
}

func showcaseGames(kind string, games []library.Game) []GameRef {
	picked := make([]library.Game, 0, len(games))
	var less func(a, b library.Game) bool
	switch kind {
	case "favorites":
		for _, g := range games {
			if g.Favorite {
				picked = append(picked, g)
			}
		}
		less = func(a, b library.Game) bool { return timeOf(a.LastPlayed).After(timeOf(b.LastPlayed)) }
	case "recently_completed":
		for _, g := range games {
			if g.Status == library.StatusCompleted {
				picked = append(picked, g)
			}
		}
		less = func(a, b library.Game) bool { return timeOf(a.StatusAt).After(timeOf(b.StatusAt)) }
	case "most_played":
		for _, g := range games {
			if g.PlaytimeSeconds > 0 {
				picked = append(picked, g)
			}
		}
		less = func(a, b library.Game) bool { return a.PlaytimeSeconds > b.PlaytimeSeconds }
	default:
		return []GameRef{}
	}
	sort.SliceStable(picked, func(i, j int) bool { return less(picked[i], picked[j]) })
	if len(picked) > maxShowcase {
		picked = picked[:maxShowcase]
	}
	refs := make([]GameRef, 0, len(picked))
	for _, g := range picked {
		refs = append(refs, ref(g))
	}
	return refs
}

func timeOf(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func ref(g library.Game) GameRef {
	return GameRef{ID: g.ID, Title: g.Title, Cover: g.Cover, CanonicalGameID: g.CanonicalGameID, PlaytimeSeconds: g.PlaytimeSeconds, Status: g.Status, StatusAt: g.StatusAt}
}
