import { errorCode, hasMessage, msg } from '../i18n';
import { installErrorText } from '../install/installErrors';

/**
 * A failure of the game properties dialog in its words, by code. Go's library and emulator reasons
 * are lower-case clauses, said after the action that failed (`fallback`); the other domains'
 * messages (wrappers, Steam, game mode) are whole sentences; the rest falls to the install table,
 * then to the action.
 */
export function propertiesErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  if (/^(library|emulators)\./.test(code) && hasMessage(code)) return msg('modals.propertiesFailedReason', { action: fallback, reason: msg(code) });
  if (hasMessage(code)) return msg(code);
  return installErrorText(err, fallback);
}
