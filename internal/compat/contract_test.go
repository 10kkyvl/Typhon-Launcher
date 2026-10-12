package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Правила ниже переписаны из typhon-backend, internal/compat/report.go
// (Report.Validate), один в один. Сервер разбирает тело с
// DisallowUnknownFields и отклоняет отчёт целиком из-за одного неверного
// поля, а лаунчер повторяет отправку каждые десять минут: любое расхождение
// превращается в вечный 400 и ни одного отданного вердикта.

type backendReport struct {
	ClientID   string        `json:"client_id"`
	AppVersion string        `json:"app_version"`
	Env        backendEnv    `json:"env"`
	Games      []backendGame `json:"games"`
}

type backendEnv struct {
	OSVersion string `json:"os_version"`
	CrossOver string `json:"crossover"`
	Chip      string `json:"chip"`
}

type backendGame struct {
	GameID   string `json:"game_id"`
	Repacker string `json:"repacker"`
	Version  string `json:"version"`
	State    string `json:"state"`
	Reason   string `json:"reason"`
}

const (
	backendMaxReportGames  = 500
	backendMaxRequestBytes = 64 << 10
)

var (
	backendRepackers = map[string]bool{
		"fitgirl": true, "dodi": true, "elamigos": true,
		"xatab": true, "kaoskrew": true, "masquerade": true,
		"unknown": true,
	}
	backendChips = map[string]bool{
		"apple_m1": true, "apple_m2": true, "apple_m3": true,
		"apple_m4": true, "apple_m5": true,
		"intel": true, "unknown": true,
	}
	backendReasons = map[string]bool{
		"launch_failed": true, "runtime_failed": true,
		"executable_missing": true, "self_exit": true, "unknown": true,
	}

	backendGameID     = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
	backendVersion    = regexp.MustCompile(`^[0-9][0-9A-Za-z._-]{0,31}$`)
	backendCrossOver  = regexp.MustCompile(`^[0-9]{1,4}(\.[0-9]{1,4}){0,2}$`)
	backendOSVersion  = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}$`)
	backendAppVersion = regexp.MustCompile(`^[0-9A-Za-z.+-]{1,32}$`)
)

func backendDecode(body []byte) (backendReport, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var r backendReport
	if err := dec.Decode(&r); err != nil {
		return backendReport{}, fmt.Errorf("decode: %w", err)
	}
	return r, nil
}

func (r backendReport) validate() error {
	if _, err := uuid.Parse(r.ClientID); err != nil {
		return errors.New("client_id")
	}
	if !backendAppVersion.MatchString(r.AppVersion) {
		return errors.New("app_version")
	}
	if !backendOSVersion.MatchString(r.Env.OSVersion) {
		return errors.New("env.os_version")
	}
	if r.Env.CrossOver != "" && !backendCrossOver.MatchString(r.Env.CrossOver) {
		return errors.New("env.crossover")
	}
	if !backendChips[r.Env.Chip] {
		return errors.New("env.chip")
	}
	if len(r.Games) == 0 {
		return errors.New("games")
	}
	if len(r.Games) > backendMaxReportGames {
		return errors.New("games: too many")
	}
	seen := make(map[string]bool, len(r.Games))
	for i, g := range r.Games {
		if err := g.validate(); err != nil {
			return fmt.Errorf("games[%d]: %w", i, err)
		}
		key := g.GameID + "\x00" + g.Repacker + "\x00" + g.Version
		if seen[key] {
			return fmt.Errorf("games[%d] duplicates an earlier entry", i)
		}
		seen[key] = true
	}
	return nil
}

func (g backendGame) validate() error {
	if !backendGameID.MatchString(g.GameID) {
		return errors.New("game_id")
	}
	if !backendRepackers[g.Repacker] {
		return errors.New("repacker")
	}
	if g.Version != "" && !backendVersion.MatchString(g.Version) {
		return errors.New("version")
	}
	switch g.State {
	case "works":
		if g.Reason != "" {
			return errors.New("reason on a working game")
		}
	case "broken":
		if g.Reason != "" && !backendReasons[g.Reason] {
			return errors.New("reason")
		}
	default:
		return errors.New("state")
	}
	return nil
}

type contractServer struct {
	mu       sync.Mutex
	accepted []backendReport
	rejected []string
}

func newContractServer(t *testing.T) (*httptest.Server, *contractServer) {
	t.Helper()
	cs := &contractServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		if len(body) > backendMaxRequestBytes {
			cs.mu.Lock()
			cs.rejected = append(cs.rejected, fmt.Sprintf("body of %d bytes exceeds %d", len(body), backendMaxRequestBytes))
			cs.mu.Unlock()
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		report, err := backendDecode(body)
		if err == nil {
			err = report.validate()
		}
		cs.mu.Lock()
		defer cs.mu.Unlock()
		if err != nil {
			cs.rejected = append(cs.rejected, fmt.Sprintf("%v\n%.200s", err, body))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		cs.accepted = append(cs.accepted, report)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, cs
}

func (cs *contractServer) snapshot() ([]backendReport, []string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return append([]backendReport(nil), cs.accepted...), append([]string(nil), cs.rejected...)
}

var (
	macEnv     = Env{OSVersion: "macOS 26.6.2", CrossOver: "26.3", Chip: "Apple M4 Pro"}
	windowsEnv = Env{OSVersion: "Windows 10 Pro (build 19045)", Chip: "AMD Ryzen 5 3600 6-Core Processor"}
)

// Всё, что лаунчер отправляет, обязано пройти проверку сервера. Окружение,
// из которого отчёт не собрать (Windows: версии macOS нет вовсе), не отправляет
// ничего, а не отправляет заведомо отклоняемое.
func TestSentReportsPassBackendValidation(t *testing.T) {
	works := func(j *Service, id string) { j.RecordSession(id, 30*time.Minute, true) }
	broken := func(j *Service, id string) {
		j.RecordLaunchFailure(id, "library.launch_failed", "x")
		j.RecordLaunchFailure(id, "library.launch_failed", "x")
	}

	cases := []struct {
		name      string
		env       Env
		record    func(*Service)
		builds    map[string]Build
		wantGames []string
	}{
		{
			name:      "macOS",
			env:       macEnv,
			record:    func(j *Service) { works(j, "g1"); broken(j, "g2") },
			builds:    map[string]Build{"g1": {GameID: "376206", Version: "1.0.16"}, "g2": {GameID: "25714", Repacker: "fitgirl"}},
			wantGames: []string{"376206", "25714"},
		},
		{
			name:   "Windows",
			env:    windowsEnv,
			record: func(j *Service) { works(j, "g1"); works(j, "g2") },
			builds: map[string]Build{"g1": {GameID: "376206", Version: "1.0.16"}, "g2": {GameID: "25714"}},
		},
		{
			name:   "macOS version unreadable",
			env:    Env{OSVersion: "darwin", Chip: "Apple M1"},
			record: func(j *Service) { works(j, "g1") },
			builds: map[string]Build{"g1": {GameID: "376206"}},
		},
		{
			name:      "one build installed twice",
			env:       macEnv,
			record:    func(j *Service) { works(j, "g1"); works(j, "g2") },
			builds:    map[string]Build{"g1": {GameID: "376206"}, "g2": {GameID: "376206"}},
			wantGames: []string{"376206"},
		},
		{
			name:   "one build with contradicting verdicts",
			env:    macEnv,
			record: func(j *Service) { works(j, "g1"); broken(j, "g2"); works(j, "g3") },
			builds: map[string]Build{
				"g1": {GameID: "376206", Repacker: "dodi"},
				"g2": {GameID: "376206", Repacker: "dodi"},
				"g3": {GameID: "25714"},
			},
			wantGames: []string{"25714"},
		},
		{
			name:      "game id longer than the server stores",
			env:       macEnv,
			record:    func(j *Service) { works(j, "g1"); works(j, "g2") },
			builds:    map[string]Build{"g1": {GameID: "1234567890123456789"}, "g2": {GameID: "25714"}},
			wantGames: []string{"25714"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, cs := newContractServer(t)
			dir := t.TempDir()
			journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
			if err != nil {
				t.Fatalf("NewServiceAt: %v", err)
			}
			tc.record(journal)
			sharer, err := NewSharer(journal, NewStatsAt(filepath.Join(dir, "compat-stats.json")), srv.URL,
				"3f1c2b8e-4d5a-4f6b-9c7d-8e9f0a1b2c3d", "0.7.2",
				func() bool { return true },
				func() (Env, error) { return tc.env, nil },
				func(localID string) (Build, bool) {
					b, ok := tc.builds[localID]
					return b, ok
				})
			if err != nil {
				t.Fatalf("NewSharer: %v", err)
			}

			sharer.sendOnce(context.Background())

			accepted, rejected := cs.snapshot()
			for _, r := range rejected {
				t.Errorf("сервер отклонил отчёт: %s", r)
			}
			if tc.wantGames == nil {
				if len(accepted) != 0 {
					t.Fatalf("отправлено %d отчётов, want 0: %+v", len(accepted), accepted)
				}
				return
			}
			if len(accepted) != 1 {
				t.Fatalf("принято %d отчётов, want 1", len(accepted))
			}
			got := make(map[string]bool, len(accepted[0].Games))
			for _, g := range accepted[0].Games {
				got[g.GameID] = true
			}
			if len(got) != len(tc.wantGames) {
				t.Fatalf("игры в отчёте %+v, want %v", accepted[0].Games, tc.wantGames)
			}
			for _, id := range tc.wantGames {
				if !got[id] {
					t.Fatalf("игры в отчёте %+v, want %v", accepted[0].Games, tc.wantGames)
				}
			}
		})
	}
}

func TestReportLimitsDoNotExceedTheBackend(t *testing.T) {
	if maxReportGames > backendMaxReportGames {
		t.Fatalf("maxReportGames = %d, сервер принимает не больше %d", maxReportGames, backendMaxReportGames)
	}
	if maxReportBytes > backendMaxRequestBytes {
		t.Fatalf("maxReportBytes = %d, сервер принимает не больше %d", maxReportBytes, backendMaxRequestBytes)
	}
}

var wideVersion = "1.0.0-" + strings.Repeat("a", 26)

// fillJournal кладёт в журнал n игр напрямую: через RecordSession каждая запись
// сбрасывала бы на диск весь журнал заново. wide выбирает самую длинную
// строку отчёта, какую принимает сервер, — с ней лимит по байтам наступает
// раньше лимита по числу игр.
func fillJournal(j *Service, n int, wide bool) map[string]Build {
	builds := make(map[string]Build, n)
	for i := range n {
		local := "g" + strconv.Itoa(i)
		build := Build{GameID: strconv.Itoa(100000000000000000 + i)}
		record := &Record{GameID: local, Attempts: 1, BestSeconds: 3600}
		if wide {
			record = &Record{GameID: local, Attempts: 2, Failures: 2, LastCode: "library.executable_missing"}
			build.Repacker = "masquerade"
			build.Version = wideVersion
		}
		j.records[local] = record
		builds[local] = build
	}
	return builds
}

// Журнал побольше лимитов сервера раньше уходил одним запросом, сервер
// отвечал на него 400 или 413, а лаунчер повторял тот же отказ каждые десять
// минут. Теперь он уходит несколькими запросами, и каждый проходит ту же
// проверку, что на сервере, а вместе они несут все игры.
func TestLargeJournalIsSentInRequestsTheBackendAccepts(t *testing.T) {
	cases := []struct {
		name        string
		games       int
		wide        bool
		minRequests int
	}{
		{"501 игра", 501, false, 2},
		{"1000 игр", 1000, false, 2},
		{"500 длинных игр не влезают в лимит по байтам", 500, true, 2},
		{"1200 длинных игр", 1200, true, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, cs := newContractServer(t)
			dir := t.TempDir()
			journal, err := NewServiceAt(filepath.Join(dir, "compat.json"))
			if err != nil {
				t.Fatalf("NewServiceAt: %v", err)
			}
			builds := fillJournal(journal, tc.games, tc.wide)
			sharer, err := NewSharer(journal, NewStatsAt(filepath.Join(dir, "compat-stats.json")), srv.URL,
				"3f1c2b8e-4d5a-4f6b-9c7d-8e9f0a1b2c3d", "0.7.2",
				func() bool { return true },
				func() (Env, error) { return macEnv, nil },
				func(localID string) (Build, bool) {
					b, ok := builds[localID]
					return b, ok
				})
			if err != nil {
				t.Fatalf("NewSharer: %v", err)
			}

			sharer.sendOnce(context.Background())

			accepted, rejected := cs.snapshot()
			for _, r := range rejected {
				t.Errorf("сервер отклонил запрос: %s", r)
			}
			if len(accepted) < tc.minRequests {
				t.Fatalf("запросов %d, want не меньше %d", len(accepted), tc.minRequests)
			}
			if !tc.wide && len(accepted) != (tc.games+backendMaxReportGames-1)/backendMaxReportGames {
				t.Fatalf("запросов %d на %d игр", len(accepted), tc.games)
			}
			seen := make(map[string]bool, tc.games)
			for _, r := range accepted {
				if r.ClientID != accepted[0].ClientID || r.Env != accepted[0].Env {
					t.Fatalf("части отчёта разошлись в общей части: %+v и %+v", accepted[0].Env, r.Env)
				}
				for _, g := range r.Games {
					if seen[g.GameID] {
						t.Fatalf("игра %s ушла дважды", g.GameID)
					}
					seen[g.GameID] = true
				}
			}
			if len(seen) != tc.games {
				t.Fatalf("на сервер дошло %d игр из %d", len(seen), tc.games)
			}

			sharer.sendOnce(context.Background())
			if again, _ := cs.snapshot(); len(again) != len(accepted) {
				t.Fatalf("неизменившийся журнал ушёл снова: %d запросов вместо %d", len(again), len(accepted))
			}
		})
	}
}
