package download

import (
	"sync"

	"typhon/internal/library"
	"typhon/internal/settings"
)

const eventPlayThrottle = "downloads:play-throttle"

type PlayThrottleState struct {
	Active        bool   `json:"active"`
	Mode          string `json:"mode"`
	DownloadLimit int64  `json:"downloadLimit"`
	UploadLimit   int64  `json:"uploadLimit"`
}

// Runs under the library mutex: takes only m.mu and calls nothing outside the package.
//
//wails:ignore
func (m *Manager) SetGamesRunning(running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.gamesRunning == running {
		return
	}
	m.gamesRunning = running
	m.syncPlayLocked()
}

func (m *Manager) PlayThrottle() PlayThrottleState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.playStateLocked()
}

func (m *Manager) playStateLocked() PlayThrottleState {
	mode := m.rateCfg.PlayDownloadMode
	if !m.gamesRunning || (mode != settings.PlayDownloadPause && mode != settings.PlayDownloadLimit) {
		return PlayThrottleState{Mode: settings.PlayDownloadOff}
	}
	state := PlayThrottleState{Active: true, Mode: mode}
	if mode == settings.PlayDownloadLimit {
		state.DownloadLimit, state.UploadLimit = m.rateCfg.PlayRates(true)
	}
	return state
}

func (m *Manager) playPausedLocked() bool {
	return m.gamesRunning && m.rateCfg.PlayDownloadMode == settings.PlayDownloadPause
}

func (m *Manager) syncPlayLocked() {
	m.applyRatesLocked()
	for _, d := range m.items {
		eng := m.engines[d.ID]
		if eng == nil {
			continue
		}
		if m.playPausedLocked() {
			eng.disallowDownload()
			eng.disallowUpload()
			continue
		}
		if m.jobs[d.ID] != nil {
			continue
		}
		switch {
		case d.Status == StatusDownloading:
			eng.allowDownload()
			applyUpload(eng, m.rateCfg.UploadWhileDownloading)
		case d.Status == StatusCompleted && d.Seeding:
			eng.allowUpload()
		}
	}
	state := m.playStateLocked()
	if state != m.playShown {
		m.playShown = state
		emit(eventPlayThrottle, state)
	}
}

func (m *Manager) applyRatesLocked() {
	if m.client == nil {
		return
	}
	m.client.applyLimits(m.rateCfg.PlayRates(m.gamesRunning))
}

func (m *Manager) allowDownloadLocked(eng engineTorrent) {
	if m.playPausedLocked() {
		return
	}
	eng.allowDownload()
}

func (m *Manager) allowUploadLocked(eng engineTorrent) {
	if m.playPausedLocked() {
		return
	}
	eng.allowUpload()
}

func (m *Manager) applyUploadLocked(eng engineTorrent, allow bool) {
	applyUpload(eng, allow && !m.playPausedLocked())
}

type PlayWatcher struct {
	mu        sync.Mutex
	running   map[string]struct{}
	listeners []func(bool)
}

//wails:ignore
func (m *Manager) PlayWatcher() *PlayWatcher {
	w := &PlayWatcher{running: map[string]struct{}{}}
	w.listeners = append(w.listeners, m.SetGamesRunning)
	return w
}

func (w *PlayWatcher) Listen(fn func(running bool)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.listeners = append(w.listeners, fn)
}

func (w *PlayWatcher) SessionStarted(game library.Game) {
	w.mu.Lock()
	defer w.mu.Unlock()
	was := len(w.running)
	w.running[game.ID] = struct{}{}
	if was == 0 {
		w.notifyLocked(true)
	}
}

func (w *PlayWatcher) SessionStopped(gameID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.running[gameID]; !ok {
		return
	}
	delete(w.running, gameID)
	if len(w.running) == 0 {
		w.notifyLocked(false)
	}
}

func (w *PlayWatcher) notifyLocked(running bool) {
	for _, fn := range w.listeners {
		fn(running)
	}
}
