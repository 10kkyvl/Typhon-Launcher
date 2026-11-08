package remote

import (
	"strings"

	"typhon/internal/account"
	"typhon/internal/download"
	"typhon/internal/sources"
	"typhon/internal/uierr"
)

const (
	actionSearch = "search"
	actionAdd    = "add"

	// Codes of the add action the bot turns into a sentence.
	codeAddOff         = "remote_downloads_disabled"
	codeReleaseMissing = "release_not_found"
	codeAlreadyThere   = "already_downloading"
	codeQueueFull      = "queue_full"
	codeAddUnavailable = "add_unavailable"
	// Codes of a background add that only the bot shows.
	codeNoDestination = "no_destination"
	codeStartFailed   = "start_failed"

	// maxFound bounds a search answer; Discord shows 25 options in a menu.
	maxFound = 10
	// maxOpenForAdd refuses an add while this many downloads are unfinished,
	// so a stolen Discord account cannot pile up the queue.
	maxOpenForAdd = 10
)

// Releases is the part of the sources service remote control may use.
type Releases interface {
	SearchDownloadable(query string, limit int) []sources.ReleaseView
	// PrepareDownloadable refuses any release SearchDownloadable would not
	// offer, so a release id from Discord cannot reach a disabled source, a
	// removed or ignored release or a patch.
	PrepareDownloadable(releaseID string) (sources.DownloadRequest, error)
}

// Starter is the part of the download manager that starts a download.
type Starter interface {
	FetchMetadata(source string) (download.TorrentInfo, error)
	StartDownloadFrom(infoHash, destination string, selectedIndices []int, origin download.Origin) (download.Download, error)
	DiscardMetadata(infoHash string)
}

// FoundRelease is one search result as the bot shows it.
type FoundRelease struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Version  string `json:"version,omitempty"`
	Repacker string `json:"repacker,omitempty"`
	Source   string `json:"source"`
	Size     int64  `json:"size,omitempty"`
	Platform string `json:"platform,omitempty"`
}

type searchResult struct {
	Releases []FoundRelease `json:"releases"`
}

type addResult struct {
	Name string `json:"name"`
}

func (s *Service) addReady() bool {
	return s.d.Releases != nil && s.d.Starter != nil && s.d.DownloadsPath != nil && s.d.AddEnabled != nil
}

func (s *Service) search(target string) account.RemoteAnswer {
	if !s.addReady() {
		return failed(codeAddUnavailable)
	}
	// Search exists to pick something to add; without permission to add it
	// would only list the sources to whoever holds the Discord account.
	if !s.d.AddEnabled() {
		return failed(codeAddOff)
	}
	found := s.d.Releases.SearchDownloadable(target, maxFound)
	res := searchResult{Releases: make([]FoundRelease, 0, len(found))}
	for _, v := range found {
		r := v.Release
		title := r.Title
		if v.GameTitle != "" {
			title = v.GameTitle
		}
		if title == "" {
			title = r.RawTitle
		}
		size := r.Size
		if r.SizeUnknown {
			size = 0
		}
		res.Releases = append(res.Releases, FoundRelease{
			ID: r.ID, Title: title, Version: r.Version, Repacker: r.Repacker,
			Source: v.SourceName, Size: size, Platform: r.Platform,
		})
	}
	return done(res)
}

// add checks what can be checked at once and answers; fetching the torrent
// takes up to 90 s, longer than the bot waits, so the download starts in the
// background and the owner learns the outcome as a notification.
func (s *Service) add(all []download.Download, releaseID string) account.RemoteAnswer {
	if !s.addReady() {
		return failed(codeAddUnavailable)
	}
	if !s.d.AddEnabled() {
		return failed(codeAddOff)
	}
	releaseID = strings.TrimSpace(releaseID)
	if releaseID == "" {
		return failed(codeReleaseMissing)
	}
	req, err := s.d.Releases.PrepareDownloadable(releaseID)
	if err != nil {
		return failed(codeReleaseMissing)
	}
	// Adds still fetching their torrent are not in the queue yet; they count
	// against the limit and the duplicate check under the same lock that
	// records them, so a burst of adds cannot pass a check taken before the
	// others were recorded.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return failed(codeAddUnavailable)
	}
	if s.adding[req.ReleaseID] {
		return failed(codeAlreadyThere)
	}
	open := len(s.adding)
	for _, d := range all {
		if d.Status == download.StatusCompleted || d.Status == download.StatusFailed {
			continue
		}
		open++
		if d.Origin.ReleaseID == req.ReleaseID {
			return failed(codeAlreadyThere)
		}
	}
	if open >= maxOpenForAdd {
		return failed(codeQueueFull)
	}
	if s.adding == nil {
		s.adding = map[string]bool{}
	}
	s.adding[req.ReleaseID] = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.adding, req.ReleaseID)
			s.mu.Unlock()
		}()
		s.startDownload(req)
	}()
	return done(addResult{Name: req.Name})
}

func (s *Service) startDownload(req sources.DownloadRequest) {
	info, err := s.d.Starter.FetchMetadata(req.URI)
	if err != nil {
		s.addFailed(req.Name, err)
		return
	}
	if s.ctx.Err() != nil {
		s.d.Starter.DiscardMetadata(info.InfoHash)
		return
	}
	// The permission may have been withdrawn while the torrent was fetched.
	if !s.d.Enabled() || !s.d.AddEnabled() {
		s.d.Starter.DiscardMetadata(info.InfoHash)
		s.d.Log.Info("remote download dropped: permission withdrawn", "component", "remote")
		s.notify(KindAddFailed, req.Name, codeAddOff)
		return
	}
	destination := s.d.DownloadsPath()
	if destination == "" {
		s.d.Starter.DiscardMetadata(info.InfoHash)
		s.d.Log.Warn("remote download did not start: no downloads folder", "component", "remote")
		s.notify(KindAddFailed, req.Name, codeNoDestination)
		return
	}
	selected := make([]int, len(info.Files))
	for i := range selected {
		selected[i] = i
	}
	origin := download.Origin{
		ReleaseID: req.ReleaseID, SourceID: req.SourceID, DistributionID: req.DistributionID,
		ReleaseUploadedAt: req.ReleaseUploadedAt, GameID: req.GameID, Version: req.Version,
	}
	if _, err := s.d.Starter.StartDownloadFrom(info.InfoHash, destination, selected, origin); err != nil {
		s.d.Starter.DiscardMetadata(info.InfoHash)
		s.addFailed(req.Name, err)
		return
	}
	s.d.Log.Info("remote download started", "component", "remote", "release", req.ReleaseID)
	s.notify(KindAddStarted, req.Name, "")
}

func (s *Service) addFailed(name string, err error) {
	// Launcher error codes travel to the bot as they are; an error without
	// one becomes the bot's generic sentence.
	code := uierr.Code(err)
	if code == "" {
		code = codeStartFailed
	}
	s.d.Log.Warn("remote download did not start", "component", "remote", "code", code, "error", err)
	s.notify(KindAddFailed, name, code)
}

func (s *Service) notify(kind, title, code string) {
	if s.d.Notify != nil {
		s.d.Notify(kind, title, code)
	}
}
