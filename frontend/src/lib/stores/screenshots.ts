import { writable } from 'svelte/store';
import { onScreenshotFailed, onScreenshotSaved, type Saved } from '../services/screenshots';
import { errorCode, hasMessage, msg } from '../i18n';
import { toast } from './toasts';

export const lastSavedShot = writable<Saved | null>(null);

export function screenshotErrorText(err: unknown, fallback: string = msg('screenshots.actionError')): string {
  const code = errorCode(err);
  return code && hasMessage(code) ? msg(code) : fallback;
}

export function initScreenshots(): () => void {
  const offSaved = onScreenshotSaved((saved) => {
    lastSavedShot.set(saved);
    toast(msg('screenshots.saved', { name: saved.title || saved.name }), 'success');
  });
  const offFailed = onScreenshotFailed((failed) => {
    const code = errorCode(failed.error);
    const reason = code && hasMessage(code) ? msg(code) : '';
    toast(reason ? msg('screenshots.failed', { reason }) : msg('screenshots.failedPlain'), 'danger');
  });
  return () => {
    offSaved();
    offFailed();
  };
}
