import { errorCode, hasMessage, msg } from '../i18n';

export function libraryErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}

// The emulator reasons that are whole sentences (they say what to do); the rest of the two domains'
// are lower-case clauses. errors.test.ts holds this list to the catalog's words.
const SENTENCES = new Set([
  'emulators.already_running',
  'emulators.bios_dir_unknown',
  'emulators.bios_missing',
  'emulators.pkg_installing',
  'emulators.pkg_invalid',
]);

/** Whether Go's reason for a code is a clause, which alone does not say what failed. */
export function isClauseCode(code: string): boolean {
  return /^(library|emulators)\./.test(code) && !SENTENCES.has(code);
}

/**
 * A failure of `action` in words, by code: a clause reason is said after the action
 * ("{action}: {reason}"), a sentence alone, and a code with no words falls to the action.
 */
export function actionErrorText(err: unknown, action: string): string {
  const code = errorCode(err);
  if (!hasMessage(code)) return action;
  return isClauseCode(code) ? msg('common.failedReason', { action, reason: msg(code) }) : msg(code);
}
