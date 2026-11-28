import { errorCode, hasMessage } from '../i18n';
import { installErrorText } from '../install/installErrors';
import { actionErrorText } from '../library/errors';

/**
 * A failure of the game properties dialog in its words, by code (`actionErrorText`: Go's library and
 * emulator clauses after the action that failed, `fallback`, the other domains' sentences alone); a
 * code with no words falls to the install table, then to the action.
 */
export function propertiesErrorText(err: unknown, fallback: string): string {
  if (hasMessage(errorCode(err))) return actionErrorText(err, fallback);
  return installErrorText(err, fallback);
}
