package library

import (
	"context"
	"log/slog"

	"typhon/internal/history"
)

type ExternalRef struct {
	Provider string
	AppID    string
}

// ExternalRefs возвращает ключи всех внешних игр библиотеки, включая
// помеченные как не установленные.
//
//wails:ignore
func (s *Service) ExternalRefs() []ExternalRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ExternalRef
	for i := range s.games {
		if ext := s.games[i].External; ext != nil {
			out = append(out, ExternalRef{Provider: ext.Provider, AppID: ext.AppID})
		}
	}
	return out
}

// SyncExternal делает то же, что ApplyExternal, и в той же операции помечает
// не установленными внешние игры провайдеров из present, которых в нём нет.
// present[provider] — все идентификаторы, которые сканер этого провайдера
// видел целиком; провайдера без записи в present не трогаем.
//
//wails:ignore
func (s *Service) SyncExternal(ctx context.Context, items []ExternalGame, present map[string]map[string]bool) (ExternalSummary, error) {
	summary, dropped, libCtx, err := s.applyExternal(ctx, items, present)
	if err != nil {
		return ExternalSummary{}, err
	}
	for _, id := range dropped {
		s.dropSteam(libCtx, id)
	}
	return summary, nil
}

func (s *Service) markGoneLocked(present map[string]map[string]bool) []Game {
	if len(present) == 0 {
		return nil
	}
	var gone []Game
	for i := range s.games {
		g := &s.games[i]
		if g.External == nil || g.Uninstalled {
			continue
		}
		seen, ok := present[g.External.Provider]
		if !ok || seen[g.External.AppID] {
			continue
		}
		if s.running[g.ID] != nil || s.starting[g.ID] != nil {
			continue
		}
		gone = append(gone, *g)
		s.resetInstallLocked(g)
	}
	return gone
}

func (s *Service) dropGoneShortcutsLocked(gone []Game) bool {
	changed := false
	for _, g := range gone {
		if g.ShortcutPath == "" {
			continue
		}
		cur := s.findLocked(g.ID)
		if cur == nil {
			continue
		}
		s.dropShortcutLocked(cur)
		if cur.ShortcutPath == "" {
			changed = true
		}
	}
	return changed
}

func (s *Service) recordUninstalledLocked(previous Game) {
	if s.historyRecord == nil {
		return
	}
	if err := s.historyRecord(history.Record{
		Kind:       history.KindUninstalled,
		GameID:     previous.ID,
		Title:      previous.Title,
		Bytes:      previous.SizeBytes,
		BytesKnown: !previous.SizeUnknown,
	}); err != nil {
		slog.Error("record history", "kind", history.KindUninstalled, "id", previous.ID, "error", err)
	}
}
