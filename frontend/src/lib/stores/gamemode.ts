import { Events } from '@wailsio/runtime';
import { Service as GameModeService } from '../../../bindings/typhon/internal/gamemode';
import { inWails } from '../services/backend';
import { gameModeErrorText, type GameModeErrorInfo } from '../gamemode/gamemode';
import { toast } from './toasts';

export const DEDUPE_MS = 10_000;

const lastShown = new Map<string, number>();

export function reportGameModeError(info: GameModeErrorInfo | null | undefined, now: number = Date.now()) {
  if (!info) return;
  const key = `${info.gameId ?? ''}|${info.code}`;
  const previous = lastShown.get(key);
  if (previous !== undefined && now - previous < DEDUPE_MS) return;
  lastShown.set(key, now);
  toast(gameModeErrorText(info.code), 'danger');
}

export function resetGameModeNotices() {
  lastShown.clear();
}

export async function reportGameModeStatus(
  load: () => Promise<{ blocked: boolean; code?: string }>,
  now: number = Date.now(),
) {
  try {
    const status = await load();
    if (status.blocked) reportGameModeError({ toggle: 'marker', code: status.code ?? '', detail: '' }, now);
  } catch (err) {
    console.error('game mode status', err);
    toast(gameModeErrorText(''), 'danger');
  }
}

export function initGameModeNotices() {
  if (!inWails) return;
  Events.On('gamemode:error', (event) => reportGameModeError(event.data as GameModeErrorInfo));
  void reportGameModeStatus(() => GameModeService.Status());
}
