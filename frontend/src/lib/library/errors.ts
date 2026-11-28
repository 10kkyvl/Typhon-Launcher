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

const same = (a: string, b: string) => a.trim().replace(/\.$/, '').toLocaleLowerCase() === b.trim().replace(/\.$/, '').toLocaleLowerCase();

/**
 * A failure of `action` in words, by code: a clause reason is said after the action
 * ("{action}: {reason}"), a sentence alone, and a code with no words falls to the action. A reason
 * that says what the action says is said once; one that holds a colon of its own is its own sentence
 * after the action ("{action}. {Reason}"), so no two colons stack.
 */
export function actionErrorText(err: unknown, action: string): string {
  const code = errorCode(err);
  if (!hasMessage(code)) return action;
  const reason = msg(code);
  if (!isClauseCode(code)) return reason;
  if (same(reason, action)) return action;
  if (reason.includes(':')) {
    return msg('common.failedReasonSentence', { action: action.replace(/\.$/, ''), reason: reason.charAt(0).toLocaleUpperCase() + reason.slice(1) });
  }
  return msg('common.failedReason', { action, reason });
}
