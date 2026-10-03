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

export function onOverlayEvent(name: 'overlay:shown' | 'overlay:hidden', handler: () => void): () => void {
  if (!inWails) return () => {};
  return Events.On(name, () => handler());
}

export async function hideOverlay(): Promise<void> {
  if (!inWails) return;
  try {
    await Events.Emit('overlay:hide');
  } catch {
    await OverlayService.Hide();
  }
}

export interface BrowserArea {
  x: number;
  y: number;
  width: number;
  height: number;
}

export function browserArea(element: HTMLElement): BrowserArea {
  const box = element.getBoundingClientRect();
  const scale = window.devicePixelRatio || 1;
  const x = Math.round(box.left * scale);
  const y = Math.round(box.top * scale);
  return {
    x,
    y,
    width: Math.round(box.right * scale) - x,
    height: Math.round(box.bottom * scale) - y,
  };
}

export async function openBrowser(address: string, area: BrowserArea): Promise<string> {
  return OverlayService.OpenBrowser(address, area);
}

export async function placeBrowser(area: BrowserArea): Promise<void> {
  await OverlayService.PlaceBrowser(area);
}

export async function closeBrowser(): Promise<void> {
  await OverlayService.CloseBrowser();
}

export async function browserBack(): Promise<void> {
  await OverlayService.BrowserBack();
}

export async function browserForward(): Promise<void> {
  await OverlayService.BrowserForward();
}

export async function browserReload(): Promise<void> {
  await OverlayService.BrowserReload();
}
