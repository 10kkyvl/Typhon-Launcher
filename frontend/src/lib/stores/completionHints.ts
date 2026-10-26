import { get } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { hintKey, shouldSuggestMastered, shouldSuggestPlaying } from '../game/hints';
import { msg } from '../i18n';
import { fetchAchievements, onAchievementsUnlocked } from '../services/achievements';
import { inWails } from '../services/backend';
import { setStatus, type CrashEvent, type SessionEvent } from '../services/library';
import { libraryErrorText } from '../library/errors';
import { libraryGames } from './library';
import { toast } from './toasts';

const shown = new Set<string>();

export const PLAYING_HINT_DELAY_MS = 60_000;
const pending = new Map<string, ReturnType<typeof setTimeout>>();

function find(gameId: string) {
  return get(libraryGames).find((game) => game.id === gameId);
}

async function mark(gameId: string, status: 'playing' | 'mastered') {
  try {
    await setStatus(gameId, status);
  } catch (err) {
    toast(libraryErrorText(err, msg('modals.gameStatusUpdateFailed')), 'danger');
  }
}

function suggestPlaying(gameId: string) {
  const game = find(gameId);
  if (!shouldSuggestPlaying(game, gameId, shown) || !game) return;
  shown.add(hintKey('playing', gameId));
  toast(msg('games.hintStarted', { title: game.title }), 'info', {
    label: msg('games.hintStartedAction'),
    run: () => void mark(gameId, 'playing'),
  });
}

export function schedulePlayingHint(gameId: string) {
  cancelPlayingHint(gameId);
  pending.set(
    gameId,
    setTimeout(() => {
      pending.delete(gameId);
      suggestPlaying(gameId);
    }, PLAYING_HINT_DELAY_MS),
  );
}

export function cancelPlayingHint(gameId: string) {
  const timer = pending.get(gameId);
  if (timer === undefined) return;
  clearTimeout(timer);
  pending.delete(gameId);
}

async function suggestMastered(gameId: string) {
  if (shown.has(hintKey('mastered', gameId))) return;
  try {
    const view = await fetchAchievements(gameId);
    if (!shouldSuggestMastered(find(gameId), view, gameId, shown)) return;
  } catch (err) {
    console.error('check achievements for a completion hint', gameId, err);
    return;
  }
  shown.add(hintKey('mastered', gameId));
  toast(msg('games.hintAllAchievements'), 'info', {
    label: msg('games.hintAllAchievementsAction'),
    run: () => void mark(gameId, 'mastered'),
  });
}

export function initCompletionHints(): () => void {
  if (!inWails) return () => {};
  const offStarted = Events.On('game:started', (event) => {
    const { gameId } = event.data as SessionEvent;
    suggestPlaying(gameId);
  });
  const offUnlocked = onAchievementsUnlocked((event) => {
    if (event.gameId) void suggestMastered(event.gameId);
  });
  return () => {
    offStarted();
    offUnlocked();
  };
}
