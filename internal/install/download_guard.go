package install

import (
	"fmt"
	"slices"

	"typhon/internal/download"
	"typhon/internal/uierr"
)

var (
	errDownloadInUse     = uierr.New("install.download_in_use", "по этой загрузке идёт установка")
	errDownloadIsInstall = uierr.New("install.download_is_install", "в папке загрузки лежит установленная игра")
	errNoGameList        = uierr.New("install.no_game_list", "не удалось сверить загрузку со списком игр")
)

// CheckDownloadDeletable says whether the data of a download may go. It is the
// guard of download.Manager.DeleteData: a running install job reads from the
// download folder, and a game installed into that folder is the only copy of
// the game.
//
//wails:ignore
func (s *Service) CheckDownloadDeletable(d download.Download) error {
	s.mu.Lock()
	extra := slices.Clone(s.downloadGuards)
	s.mu.Unlock()
	for _, guard := range extra {
		if err := guard(d); err != nil {
			return err
		}
	}
	root := downloadRoot(d)
	hasRoot := root != ""
	if s.busyWith(d.ID, root, hasRoot) {
		return errDownloadInUse
	}
	if !hasRoot {
		return nil
	}
	if s.library == nil {
		return errNoGameList
	}
	for _, game := range s.library.GetGames() {
		if sharesPath(root, game.InstallDir) {
			return fmt.Errorf("%w: %s", errDownloadIsInstall, game.Title)
		}
	}
	return nil
}

// AddDownloadGuard registers another service's veto over deleting a download:
// the update service reads downloads that no install record mentions.
//
//wails:ignore
func (s *Service) AddDownloadGuard(guard func(download.Download) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloadGuards = append(s.downloadGuards, guard)
}

func (s *Service) busyWith(downloadID, root string, hasRoot bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.items {
		if !active(item.Status) {
			continue
		}
		if item.DownloadID == downloadID {
			return true
		}
		if hasRoot && (sharesPath(root, item.SourcePath) || sharesPath(root, item.ContentRoot) || sharesPath(root, item.Destination)) {
			return true
		}
	}
	return false
}
