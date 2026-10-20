import { hasMessage, msg } from '../i18n';
import type { GameModeOverride } from '../services/library';
import type { GameMode } from '../services/settings';

export type GameModeKey = keyof GameMode;
export type OverrideState = 'inherit' | 'on' | 'off';

export const gameModeKeys: GameModeKey[] = ['highPerformance', 'hideLauncher', 'raisePriority'];

export interface GameModeErrorInfo {
  gameId?: string;
  toggle: string;
  code: string;
  detail: string;
}

export function overrideState(o: GameModeOverride | null | undefined, key: GameModeKey): OverrideState {
  const value = o?.[key];
  if (value === true) return 'on';
  if (value === false) return 'off';
  return 'inherit';
}

export function withOverride(
  o: GameModeOverride | null | undefined,
  key: GameModeKey,
  state: OverrideState,
): GameModeOverride {
  const next: GameModeOverride = {};
  for (const k of gameModeKeys) {
    const value = k === key ? (state === 'inherit' ? null : state === 'on') : o?.[k];
    if (value === true || value === false) next[k] = value;
  }
  return next;
}

export function effectiveGameMode(global: GameMode | undefined, o: GameModeOverride | null | undefined, key: GameModeKey): boolean {
  const state = overrideState(o, key);
  if (state === 'inherit') return Boolean(global?.[key]);
  return state === 'on';
}

export function inheritLabel(globalValue: boolean): string {
  const state = msg(globalValue ? 'modals.propertiesGameModeStateOn' : 'modals.propertiesGameModeStateOff');
  return msg('modals.propertiesGameModeInherit', { state });
}

export function gameModeErrorText(code: string): string {
  return hasMessage(code) && code.startsWith('gamemode.') ? msg(code) : msg('gamemode.failed');
}
