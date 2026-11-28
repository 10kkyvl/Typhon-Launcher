import { errorCode, hasMessage, msg } from '../i18n';

export function libraryErrorText(err: unknown, fallback: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? msg(code) : fallback;
}

// The emulator reasons that are whole sentences (they say what to do); the rest of the two domains'
// are reasons that do not say what failed. errors.test.ts holds this list to the Russian words.
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

// Words that read on after a colon: they open lower-case and hold no colon and no sentence end of their own.
const readsOn = (reason: string) => /^\p{Ll}/u.test(reason) && !reason.includes(':') && !/[.!?]\s/.test(reason.trim());

function composed(action: string, reason: string): string {
  if (same(reason, action)) return action;
  if (readsOn(reason)) return msg('common.failedReason', { action, reason });
  return msg('common.failedReasonSentence', { action: action.replace(/\.$/, ''), reason: reason.charAt(0).toLocaleUpperCase() + reason.slice(1) });
}

/**
 * A failure of `action` in words, by code: a sentence code alone, and a code with no words falls to
 * the action. A clause code's reason is said after the action in the form its words in the current
 * language take: after a colon when they open lower-case and hold no colon or sentence end of their
 * own ("{action}: {reason}"), else as their own sentence ("{action}. {Reason}"); a reason that says
 * what the action says is said once.
 */
export function actionErrorText(err: unknown, action: string): string {
  const code = errorCode(err);
  if (!hasMessage(code)) return action;
  const reason = msg(code);
  if (!isClauseCode(code)) return reason;
  return composed(action, reason);
}

/**
 * A failure that arrives after its dialog is gone, in words that name its target: the action (one
 * that names the emulator or the game) is always said, followed by any code's reason in the form
 * `actionErrorText` gives a clause, a whole sentence included; a code with no words falls to the
 * action.
 */
export function lateErrorText(err: unknown, action: string): string {
  const code = errorCode(err);
  return hasMessage(code) ? composed(action, msg(code)) : action;
}
