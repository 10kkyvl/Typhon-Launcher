import { Events } from '@wailsio/runtime';
import { Service as ScreenshotsService } from '../../../bindings/typhon/internal/screenshots';
import type { Failed, Saved, Shot, Status } from '../../../bindings/typhon/internal/screenshots';
import { inWails } from './backend';

export type { Failed, Saved, Shot, Status };

export const SCREENSHOT_HOTKEYS = ['F12', 'F11', 'Shift+F12', 'Ctrl+F12'] as const;
export const DEFAULT_SCREENSHOT_HOTKEY = 'Shift+F12' satisfies (typeof SCREENSHOT_HOTKEYS)[number];

const unavailable = () => new Error('unavailable in browser');

export async function listShots(gameId: string): Promise<Shot[]> {
  if (!inWails) return [];
  return ((await ScreenshotsService.List(gameId)) ?? []) as Shot[];
}

export async function deleteShot(gameId: string, name: string): Promise<void> {
  if (!inWails) throw unavailable();
  await ScreenshotsService.Delete(gameId, name);
}

export async function openShotsFolder(gameId: string): Promise<void> {
  if (!inWails) throw unavailable();
  await ScreenshotsService.OpenFolder(gameId);
}

export async function screenshotsStatus(): Promise<Status> {
  if (!inWails) return { enabled: true, hotkey: DEFAULT_SCREENSHOT_HOTKEY, dir: '' };
  return (await ScreenshotsService.Status()) as Status;
}

export function onScreenshotSaved(handler: (saved: Saved) => void): () => void {
  if (!inWails) return () => {};
  return Events.On('screenshot:saved', (event) => handler(event.data as Saved));
}

export function onScreenshotFailed(handler: (failed: Failed) => void): () => void {
  if (!inWails) return () => {};
  return Events.On('screenshot:failed', (event) => handler(event.data as Failed));
}
