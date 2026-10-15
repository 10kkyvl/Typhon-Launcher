package playlog

import (
	"sort"
	"time"

	"typhon/internal/uierr"
)

const topLimit = 10

var errBadPeriod = uierr.New("playlog.bad_period", "неизвестный период статистики")

var periodDays = map[string]int{"week": 7, "month": 30, "year": 365}

type DayTotal struct {
	Date    string `json:"date"`
	Seconds int64  `json:"seconds"`
}

type GameTotal struct {
	GameID   string `json:"gameId"`
	Title    string `json:"title"`
	Seconds  int64  `json:"seconds"`
	Sessions int    `json:"sessions"`
}

type LongestSession struct {
	GameID  string `json:"gameId"`
	Title   string `json:"title"`
	Seconds int64  `json:"seconds"`
	Date    string `json:"date"`
}

type Stats struct {
	Period       string          `json:"period"`
	From         string          `json:"from"`
	To           string          `json:"to"`
	TotalSeconds int64           `json:"totalSeconds"`
	Sessions     int             `json:"sessions"`
	Days         []DayTotal      `json:"days"`
	Top          []GameTotal     `json:"top"`
	Longest      *LongestSession `json:"longest"`
}

func (s *Service) Stats(period string) (Stats, error) {
	length, ok := periodDays[period]
	if !ok {
		return Stats{}, errBadPeriod
	}

	s.mu.Lock()
	now := s.now()
	titleOf := s.titleOf
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	first := today.AddDate(0, 0, -(length - 1))

	result := Stats{
		Period: period,
		From:   first.Format(dayLayout),
		To:     today.Format(dayLayout),
		Days:   make([]DayTotal, length),
		Top:    []GameTotal{},
	}
	index := make(map[string]int, length)
	for i := range result.Days {
		date := first.AddDate(0, 0, i).Format(dayLayout)
		result.Days[i] = DayTotal{Date: date}
		index[date] = i
	}

	totals := map[string]*GameTotal{}
	for _, day := range s.days {
		i, inWindow := index[day.Date]
		if !inWindow {
			continue
		}
		result.Days[i].Seconds += day.Seconds
		result.TotalSeconds += day.Seconds
		result.Sessions += day.Sessions
		total := totals[day.GameID]
		if total == nil {
			total = &GameTotal{GameID: day.GameID}
			totals[day.GameID] = total
		}
		total.Seconds += day.Seconds
		total.Sessions += day.Sessions
		if day.Sessions > 0 && (result.Longest == nil || day.Longest > result.Longest.Seconds) {
			result.Longest = &LongestSession{GameID: day.GameID, Seconds: day.Longest, Date: day.Date}
		}
	}

	s.mu.Unlock()

	for _, total := range totals {
		total.Title = resolveTitle(titleOf, total.GameID)
		result.Top = append(result.Top, *total)
	}
	sort.Slice(result.Top, func(i, j int) bool {
		if result.Top[i].Seconds != result.Top[j].Seconds {
			return result.Top[i].Seconds > result.Top[j].Seconds
		}
		return result.Top[i].GameID < result.Top[j].GameID
	})
	if len(result.Top) > topLimit {
		result.Top = result.Top[:topLimit]
	}
	if result.Longest != nil {
		result.Longest.Title = resolveTitle(titleOf, result.Longest.GameID)
	}
	return result, nil
}

func resolveTitle(titleOf func(string) (string, bool), gameID string) string {
	if titleOf == nil {
		return ""
	}
	title, ok := titleOf(gameID)
	if !ok {
		return ""
	}
	return title
}
