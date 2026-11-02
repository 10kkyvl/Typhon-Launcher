package install

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"typhon/internal/download"
)

func imageOptions(origin download.Origin) StartOptions {
	return StartOptions{Platform: origin.Platform, Title: origin.Title, Cover: origin.Cover}
}

// withImageOrigin makes the console of a download binding: a caller that does not
// name it must not install a console image as a PC game. What the caller did say
// stays, and the checks that follow refuse what does not fit an image.
func withImageOrigin(opts StartOptions, origin download.Origin) StartOptions {
	if opts.Platform != "" || origin.Platform == "" {
		return opts
	}
	opts.Platform = origin.Platform
	if opts.Title == "" {
		opts.Title = origin.Title
	}
	if opts.Cover == "" {
		opts.Cover = origin.Cover
	}
	return opts
}

// checkImageOrigin refuses at the start of a download what checkImageOptions
// would refuse at its end.
func checkImageOrigin(origin download.Origin) error {
	if origin.Platform == "" && origin.Title == "" && origin.Cover == "" {
		return nil
	}
	if origin.Purpose != download.PurposeRelease {
		return fmt.Errorf("%w: образ не обновляется и не чинится загрузкой", errImageOptions)
	}
	return checkImageOptions(imageOptions(origin))
}

// installImageDownload starts the install of a finished console image. An image
// has no step that needs the player, so the auto-install setting does not apply
// and nothing asks before the copy. With no one waiting for an answer, a refusal
// goes into the list of installs, where the player sees it and can retry.
func (s *Service) installImageDownload(d download.Download) {
	if d.ImageAttempted {
		return
	}
	if s.hasInstall(d.ID) {
		s.markImageAttempted(d.ID)
		return
	}
	item, err := s.Start(d.ID, imageOptions(d.Origin))
	switch {
	case err == nil:
		slog.Info("image install started", "id", item.ID, "download", d.ID, "name", item.Name)
	case errors.Is(err, errBusy) && s.hasInstall(d.ID):
		slog.Info("image install skipped, already taken care of", "download", d.ID, "error", err)
	case imageAttemptLater(err):
		// The attempt did not take place: the next start of the launcher repeats it.
		slog.Warn("image install put off", "download", d.ID, "error", err)
		return
	default:
		if err := s.recordImageRefusal(d, err); err != nil {
			slog.Error("record refused image install", "download", d.ID, "error", err)
			return
		}
	}
	s.markImageAttempted(d.ID)
}

func imageAttemptLater(err error) bool {
	for _, later := range []error{errNotStarted, errUnavailable, errDownloadDeleting, errNotCompleted, download.ErrNotFound, context.Canceled} {
		if errors.Is(err, later) {
			return true
		}
	}
	return false
}

func (s *Service) hasInstall(downloadID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasInstallLocked(downloadID)
}

func (s *Service) hasInstallLocked(downloadID string) bool {
	for _, item := range s.items {
		if item.DownloadID == downloadID {
			return true
		}
	}
	return false
}

// markImageAttempted failing costs nothing but a look at the download on the
// next start: the install record that exists by now stops it there.
func (s *Service) markImageAttempted(downloadID string) {
	// A finished move takes the download away, possibly before this line.
	if err := s.downloads.MarkImageAttempted(downloadID); err != nil && !errors.Is(err, download.ErrNotFound) {
		slog.Error("mark image install attempted", "download", downloadID, "error", err)
	}
}

// recordImageRefusal leaves the record empty of a destination: nothing was
// chosen yet, and Retry chooses it.
func (s *Service) recordImageRefusal(d download.Download, cause error) error {
	name, cover := strings.TrimSpace(d.Origin.Title), d.Origin.Cover
	if checkImageOptions(imageOptions(d.Origin)) != nil {
		name, cover = "", ""
	}
	if name == "" {
		name = s.nameFor(d)
	}
	source, err := sourceDir(d)
	if err != nil {
		cause = errors.Join(cause, err)
	}
	item := &Installation{
		ID:         newID(),
		DownloadID: d.ID,
		Name:       name,
		Type:       TypeImage,
		Status:     StatusFailed,
		SourcePath: source,
		Origin:     d.Origin,
		Platform:   d.Origin.Platform,
		Cover:      cover,
		StartedAt:  time.Now(),
		Error:      cause.Error(),
	}
	s.mu.Lock()
	if s.hasInstallLocked(d.ID) {
		s.mu.Unlock()
		return nil
	}
	s.items = append(s.items, item)
	if err := s.persistLocked(); err != nil {
		s.items = s.items[:len(s.items)-1]
		s.mu.Unlock()
		return wrapPersistError(err)
	}
	snap := snapshotOf(item)
	s.mu.Unlock()

	slog.Warn("image install refused", "id", snap.ID, "download", d.ID, "error", cause)
	emit(eventFailed, snap)
	emit(eventUpdated, snap)
	return nil
}

// placeRefusedImage is what Start would have done for a record that never got
// that far.
func (s *Service) placeRefusedImage(item Installation, plan Plan, mode string) (string, error) {
	if err := s.checkImageNew(item); err != nil {
		return "", err
	}
	dest, err := s.imageDestination(s.config(), "", item.Platform, item.Name)
	if err != nil {
		return "", err
	}
	if s.insideGame(dest) {
		return "", fmt.Errorf("%w: %s", errDestInsideGame, dest)
	}
	if err := s.checkSpace(dest, spaceNeeded(plan, mode, s.sameVolume(plan.ContentRoot, dest))); err != nil {
		return "", err
	}
	if err := s.prepareDestination(dest, item.Name); err != nil {
		return "", err
	}
	return dest, nil
}

// resumeImageInstalls closes the gap between a download recorded as finished and
// its install recorded as started: the launcher can die there, and a completion
// can also arrive before this service is up. Downloads start first (see the
// order of Services in main.go), so the list is already read.
func (s *Service) resumeImageInstalls(ctx context.Context) {
	if s.downloads == nil {
		return
	}
	for _, d := range s.downloads.List() {
		if ctx.Err() != nil {
			return
		}
		if d.Origin.Platform != "" && d.Origin.Purpose == download.PurposeRelease && d.Status == download.StatusCompleted {
			//nolint:contextcheck // инварианты 19-20: Start берёт контекст жизни сервиса из s.ctx, его сигнатура задана биндингом и ctx не принимает
			s.installImageDownload(d)
		}
	}
}
