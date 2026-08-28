package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"typhon/internal/selfupdate"
)

const changelogDateLayout = "2006-01-02"

var (
	errChangelogEmpty       = errors.New("signrelease: changelog has no release entries")
	errChangelogHeading     = errors.New("signrelease: changelog release heading must read \"## <version> — <YYYY-MM-DD>\"")
	errChangelogSection     = errors.New("signrelease: changelog section heading is not a known change kind")
	errChangelogStray       = errors.New("signrelease: changelog line belongs to no section")
	errChangelogOrphan      = errors.New("signrelease: changelog continuation line has no bullet to continue")
	errChangelogVersionHead = errors.New("signrelease: changelog does not start with the version being released")
)

var changeKinds = map[string]selfupdate.ChangeKind{
	"добавлено":  selfupdate.ChangeAdded,
	"added":      selfupdate.ChangeAdded,
	"изменено":   selfupdate.ChangeChanged,
	"changed":    selfupdate.ChangeChanged,
	"исправлено": selfupdate.ChangeFixed,
	"fixed":      selfupdate.ChangeFixed,
	"удалено":    selfupdate.ChangeRemoved,
	"removed":    selfupdate.ChangeRemoved,
}

var headingSeparators = []string{" — ", " – ", " - "}

type changelogEntry struct {
	note    selfupdate.ReleaseNote
	kind    selfupdate.ChangeKind
	inKind  bool
	summary []string
}

func parseReleaseHeading(line string) (string, time.Time, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "##"))
	for _, sep := range headingSeparators {
		version, date, ok := strings.Cut(rest, sep)
		if !ok {
			continue
		}
		published, err := time.Parse(changelogDateLayout, strings.TrimSpace(date))
		if err != nil {
			return "", time.Time{}, fmt.Errorf("%w: %q: %w", errChangelogHeading, line, err)
		}
		return strings.TrimSpace(version), published.UTC(), nil
	}
	return "", time.Time{}, fmt.Errorf("%w: %q", errChangelogHeading, line)
}

func (e *changelogEntry) addBullet(text string) error {
	if !e.inKind {
		return fmt.Errorf("%w: %q", errChangelogStray, text)
	}
	e.note.Changes = append(e.note.Changes, selfupdate.Change{Kind: e.kind, Text: text})
	return nil
}

func (e *changelogEntry) continueBullet(text string) error {
	if !e.inKind || len(e.note.Changes) == 0 {
		return fmt.Errorf("%w: %q", errChangelogOrphan, text)
	}
	last := &e.note.Changes[len(e.note.Changes)-1]
	last.Text = last.Text + " " + text
	return nil
}

func (e *changelogEntry) finish() selfupdate.ReleaseNote {
	e.note.Summary = strings.Join(e.summary, " ")
	return e.note
}

// parseChangelog reads the Keep-a-Changelog shape the launcher repository
// writes by hand: "## <version> — <date>", an optional summary paragraph, and
// "### <kind>" sections of "- " bullets. Anything else is an error, because a
// silently skipped line is a release note the user never sees.
func parseChangelog(data []byte) ([]selfupdate.ReleaseNote, error) {
	var (
		notes   []selfupdate.ReleaseNote
		current *changelogEntry
	)

	flush := func() {
		if current != nil {
			notes = append(notes, current.finish())
			current = nil
		}
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "## "):
			flush()
			version, published, err := parseReleaseHeading(line)
			if err != nil {
				return nil, err
			}
			current = &changelogEntry{note: selfupdate.ReleaseNote{Version: version, PublishedAt: published}}
		case current == nil:
			continue
		case trimmed == "":
			continue
		case strings.HasPrefix(line, "### "):
			name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "### ")))
			kind, ok := changeKinds[name]
			if !ok {
				return nil, fmt.Errorf("%w: %q", errChangelogSection, line)
			}
			current.kind, current.inKind = kind, true
		case strings.HasPrefix(trimmed, "- "):
			if err := current.addBullet(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))); err != nil {
				return nil, err
			}
		case current.inKind:
			if err := current.continueBullet(trimmed); err != nil {
				return nil, err
			}
		default:
			current.summary = append(current.summary, trimmed)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("signrelease: read changelog: %w", err)
	}
	flush()

	if len(notes) == 0 {
		return nil, errChangelogEmpty
	}
	return notes, nil
}

func releaseNotesFor(data []byte, version string, maxReleases int) ([]selfupdate.ReleaseNote, error) {
	notes, err := parseChangelog(data)
	if err != nil {
		return nil, err
	}
	if notes[0].Version != version {
		return nil, fmt.Errorf("%w: top entry is %q, releasing %q", errChangelogVersionHead, notes[0].Version, version)
	}
	if maxReleases > 0 && len(notes) > maxReleases {
		notes = notes[:maxReleases]
	}
	return notes, nil
}
