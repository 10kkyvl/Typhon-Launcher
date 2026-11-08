// Package remote runs the commands the Discord bot leaves for this launcher.
// The backend wakes it through the chat event stream; it then fetches the
// waiting commands, runs each against the download queue and posts the
// answer. Nothing runs unless "Allow control from Discord" is on in this
// launcher, and the action whitelist mirrors the backend's: launching games
// and deleting data are not remote actions, and starting a download of a
// release from this launcher's own sources also needs "Allow starting
// downloads from Discord".
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"typhon/internal/account"
	"typhon/internal/download"
)

const (
	actionStatus = "status"
	actionPause  = "pause"
	actionResume = "resume"
	actionStart  = "start"
	actionCancel = "cancel"

	// Error codes the bot turns into a sentence.
	codeDisabled  = "remote_disabled"
	codeNotFound  = "download_not_found"
	codeAmbiguous = "download_ambiguous"
	codeNothing   = "nothing_to_do"
	codeFailed    = "action_failed"
	codeUnknown   = "unknown_action"

	requestTimeout = 15 * time.Second
	maxListed      = 25
)

// API is the backend side.
type API interface {
	RemoteCommands(ctx context.Context, installationID string) ([]account.RemoteCommand, error)
	FinishRemoteCommand(ctx context.Context, installationID, id string, a account.RemoteAnswer) error
}

// Downloads is the part of the download manager remote control may use.
type Downloads interface {
	List() []download.Download
	Pause(id string) error
	Resume(id string) error
	ForceStart(id string) error
	Cancel(id string) error
}

// Deps wires a Service.
type Deps struct {
	API            API
	Downloads      Downloads
	InstallationID string
	// Enabled reads the setting at the moment a command runs.
	Enabled func() bool
	// Running lists the titles of the games running now.
	Running func() []string
	Log     *slog.Logger

	// Releases, Starter, DownloadsPath and AddEnabled make search and add
	// work; without them both answer add_unavailable. AddEnabled reads
	// "Allow starting downloads from Discord" when a command runs.
	Releases      Releases
	Starter       Starter
	DownloadsPath func() string
	AddEnabled    func() bool
	// Notify sends a message to the owner's Discord account; nil sends none.
	Notify func(kind, title, code string)
}

type Service struct {
	d    Deps
	mu   sync.Mutex
	busy bool
	more bool
	ctx  context.Context
	wg   sync.WaitGroup
}

func New(ctx context.Context, d Deps) (*Service, error) {
	if d.API == nil || d.Downloads == nil || d.InstallationID == "" || d.Enabled == nil || d.Running == nil {
		return nil, errors.New("remote: missing dependencies")
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d, ctx: ctx}, nil
}

// Wake fetches and runs the waiting commands. Wakes that arrive while a pass
// runs fold into one more pass, so a burst of events costs one request.
func (s *Service) Wake() {
	if !s.d.Enabled() {
		return
	}
	s.mu.Lock()
	if s.busy {
		s.more = true
		s.mu.Unlock()
		return
	}
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		for {
			s.pass()
			s.mu.Lock()
			if !s.more || s.ctx.Err() != nil {
				s.busy, s.more = false, false
				s.mu.Unlock()
				return
			}
			s.more = false
			s.mu.Unlock()
		}
	}()
}

// Wait returns once no pass is running; for shutdown.
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) pass() {
	ctx, cancel := context.WithTimeout(s.ctx, requestTimeout)
	defer cancel()
	commands, err := s.d.API.RemoteCommands(ctx, s.d.InstallationID)
	if err != nil {
		s.d.Log.Warn("fetch remote commands", "component", "remote", "error", err)
		return
	}
	for _, c := range commands {
		answer := s.Run(c)
		if err := s.d.API.FinishRemoteCommand(ctx, s.d.InstallationID, c.ID, answer); err != nil {
			s.d.Log.Warn("answer remote command", "component", "remote", "action", c.Action, "error", err)
			continue
		}
		s.d.Log.Info("remote command", "component", "remote", "action", c.Action, "status", answer.Status, "code", answer.ErrorCode)
	}
}

// DownloadView is a download as the bot shows it.
type DownloadView struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	Progress      float64 `json:"progress"`
	DownloadSpeed int64   `json:"downloadSpeed,omitempty"`
	ETASeconds    int64   `json:"etaSeconds,omitempty"`
}

type statusResult struct {
	Running   []string       `json:"running"`
	Downloads []DownloadView `json:"downloads"`
	More      int            `json:"more,omitempty"`
}

type actionResult struct {
	Affected []DownloadView `json:"affected"`
}

func view(d download.Download) DownloadView {
	v := DownloadView{ID: d.ID, Name: d.Name, Status: string(d.Status), Progress: d.Progress}
	if d.Status == download.StatusDownloading {
		v.DownloadSpeed, v.ETASeconds = d.DownloadSpeed, d.ETASeconds
	}
	return v
}

func done(result any) account.RemoteAnswer {
	raw, err := json.Marshal(result)
	if err != nil {
		return failed(codeFailed)
	}
	return account.RemoteAnswer{Status: "done", Result: raw}
}

func failed(code string) account.RemoteAnswer {
	return account.RemoteAnswer{Status: "failed", ErrorCode: code}
}

// Run executes one command. It never returns an error: every outcome is an
// answer the bot can show.
func (s *Service) Run(c account.RemoteCommand) account.RemoteAnswer {
	if !s.d.Enabled() {
		return failed(codeDisabled)
	}
	all := s.d.Downloads.List()
	switch c.Action {
	case actionStatus:
		res := statusResult{Running: s.d.Running(), Downloads: []DownloadView{}}
		if res.Running == nil {
			res.Running = []string{}
		}
		for i, d := range all {
			if i == maxListed {
				res.More = len(all) - maxListed
				break
			}
			res.Downloads = append(res.Downloads, view(d))
		}
		return done(res)
	case actionPause:
		return s.apply(all, c.Target, true, pausable, s.d.Downloads.Pause)
	case actionResume:
		return s.apply(all, c.Target, true, func(d download.Download) bool { return d.Status == download.StatusPaused }, s.d.Downloads.Resume)
	case actionStart:
		return s.apply(all, c.Target, false, func(d download.Download) bool {
			return d.Status == download.StatusQueued || d.Status == download.StatusPaused
		}, s.d.Downloads.ForceStart)
	case actionSearch:
		return s.search(c.Target)
	case actionAdd:
		return s.add(all, c.Target)
	case actionCancel:
		// Cancel drops a download's progress: one named download only.
		if strings.TrimSpace(c.Target) == "" || strings.EqualFold(strings.TrimSpace(c.Target), "all") {
			return failed(codeNotFound)
		}
		return s.apply(all, c.Target, false, func(d download.Download) bool { return d.Status != download.StatusCompleted }, s.d.Downloads.Cancel)
	}
	return failed(codeUnknown)
}

func pausable(d download.Download) bool {
	switch d.Status {
	case download.StatusQueued, download.StatusMetadata, download.StatusDownloading, download.StatusVerifying:
		return true
	}
	return false
}

// apply runs act on the downloads target names. "all" (or nothing, where
// allowAll) means every download eligible accepts; anything else must name
// exactly one download, by id or by part of its name.
func (s *Service) apply(all []download.Download, target string, allowAll bool, eligible func(download.Download) bool, act func(string) error) account.RemoteAnswer {
	target = strings.TrimSpace(target)
	var chosen []download.Download
	if allowAll && (target == "" || strings.EqualFold(target, "all")) {
		for _, d := range all {
			if eligible(d) {
				chosen = append(chosen, d)
			}
		}
		if len(chosen) == 0 {
			return failed(codeNothing)
		}
	} else {
		d, code := pick(all, target)
		if code != "" {
			return failed(code)
		}
		if !eligible(d) {
			return failed(codeNothing)
		}
		chosen = []download.Download{d}
	}
	var acted []download.Download
	for _, d := range chosen {
		if err := act(d.ID); err != nil {
			s.d.Log.Warn("remote action failed", "component", "remote", "download", d.ID, "error", err)
			if len(chosen) == 1 {
				return failed(codeFailed)
			}
			continue
		}
		acted = append(acted, d)
	}
	// Report each download as it is after the action, not as it was before.
	now := map[string]download.Download{}
	for _, d := range s.d.Downloads.List() {
		now[d.ID] = d
	}
	res := actionResult{Affected: []DownloadView{}}
	for _, d := range acted {
		if fresh, ok := now[d.ID]; ok {
			d = fresh
		}
		res.Affected = append(res.Affected, view(d))
	}
	if len(res.Affected) == 0 {
		return failed(codeFailed)
	}
	return done(res)
}

func pick(all []download.Download, target string) (download.Download, string) {
	if target == "" {
		return download.Download{}, codeNotFound
	}
	for _, d := range all {
		if d.ID == target {
			return d, ""
		}
	}
	needle := strings.ToLower(target)
	var found []download.Download
	for _, d := range all {
		name := strings.ToLower(d.Name)
		if name == needle {
			return d, ""
		}
		if strings.Contains(name, needle) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return download.Download{}, codeNotFound
	case 1:
		return found[0], ""
	}
	return download.Download{}, codeAmbiguous
}
