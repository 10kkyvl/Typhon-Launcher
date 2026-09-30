import { Events } from '@wailsio/runtime';
import { Service as OverlayService } from '../../../bindings/typhon/internal/overlay';
import { inWails } from './backend';

export interface OverlayStatus {
  supported: boolean;
  enabled: boolean;
  hotkey: string;
  error: string;
}

export const DEFAULT_OVERLAY_HOTKEY = 'Alt+`';
export const OVERLAY_HOTKEYS = ['Alt+`', 'Shift+F1', 'Shift+F2', 'Ctrl+Shift+O'] as const;

export function isOverlayWindow(): boolean {
  return new URLSearchParams(window.location.search).has('overlay');
}

export function toOverlayStatus(value: unknown): OverlayStatus {
  const status = value as Partial<OverlayStatus> | null;
  return {
    supported: status?.supported === true,
    enabled: status?.enabled === true,
    hotkey: status?.hotkey || DEFAULT_OVERLAY_HOTKEY,
    error: status?.error ?? '',
  };
}

export async function overlayStatus(): Promise<OverlayStatus> {
  if (!inWails) return toOverlayStatus(null);
  return toOverlayStatus(await OverlayService.Status());
}

export function onOverlayStatus(handler: (status: OverlayStatus) => void): () => void {
  if (!inWails) return () => {};
  return Events.On('overlay:status', (event) => handler(toOverlayStatus(event.data)));
}

export interface OverlayView {
  visible: boolean;
  exclusive: boolean;
}

export interface OverlayShown {
  exclusive: boolean;
}

export async function overlayView(): Promise<OverlayView> {
  if (!inWails) return { visible: false, exclusive: false };
  const view = (await OverlayService.View()) as Partial<OverlayView> | null;
  return { visible: view?.visible === true, exclusive: view?.exclusive === true };
}

export function onOverlayEvent(name: 'overlay:shown' | 'overlay:hidden', handler: (shown: OverlayShown) => void): () => void {
  if (!inWails) return () => {};
  return Events.On(name, (event) => {
    const data = event.data as Partial<OverlayShown> | null;
    handler({ exclusive: data?.exclusive === true });
  });
}

export async function hideOverlay(): Promise<void> {
  if (!inWails) return;
  try {
    await Events.Emit('overlay:hide');
  } catch {
    await OverlayService.Hide();
  }
}
