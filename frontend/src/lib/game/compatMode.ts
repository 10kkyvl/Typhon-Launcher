import { hasMessage, msg } from '../i18n';
import type { WrapperId, WrapperInfo, WrapperProgress } from '../services/wrappers';

export const WRAPPERS: readonly WrapperId[] = ['dgvoodoo', 'dxvk'];

const PRESETS: Record<WrapperId, readonly string[]> = {
  dgvoodoo: ['default', '1080p', 'native'],
  dxvk: ['default'],
};

const DIRECTX: Record<string, { label: string; rank: number }> = {
  d3d8: { label: 'DirectX 8', rank: 8 },
  d3d9: { label: 'DirectX 9', rank: 9 },
  d3d10: { label: 'DirectX 10', rank: 10 },
  d3d10_1: { label: 'DirectX 10', rank: 10 },
  d3d10core: { label: 'DirectX 10', rank: 10 },
  d3d11: { label: 'DirectX 11', rank: 11 },
  d3d12: { label: 'DirectX 12', rank: 12 },
};

const OTHER: Record<string, string> = {
  ddraw: 'DirectDraw',
  opengl32: 'OpenGL',
  glide: 'Glide',
  glide2x: 'Glide',
  glide3x: 'Glide',
};

export function wrapperName(id: string): string {
  if (id === 'dgvoodoo') return 'dgVoodoo2';
  if (id === 'dxvk') return 'DXVK';
  return id;
}

export function describeImports(imports: readonly string[]): string {
  const names = imports.map((n) => n.toLowerCase().replace(/\.dll$/, ''));
  const labels: string[] = [];
  const add = (label: string) => {
    if (!labels.includes(label)) labels.push(label);
  };
  names
    .filter((n) => DIRECTX[n])
    .sort((a, b) => DIRECTX[a].rank - DIRECTX[b].rank)
    .forEach((n) => add(DIRECTX[n].label));
  const hasDirectX = labels.length > 0;
  for (const n of names) {
    const label = OTHER[n];
    if (!label) continue;
    if (label === 'DirectDraw' && hasDirectX) continue;
    add(label);
  }
  if (labels.length === 0) return '';
  return `${labels.join(', ')} (${names.join(', ')})`;
}

export function defaultWrapper(info: Pick<WrapperInfo, 'suggested'>): WrapperId {
  return info.suggested === 'dxvk' ? 'dxvk' : 'dgvoodoo';
}

export function presetsFor(wrapper: WrapperId): readonly string[] {
  return PRESETS[wrapper];
}

export function presetLabel(preset: string): string {
  const key = `modals.compatPreset.${preset}`;
  return hasMessage(key) ? msg(key) : preset;
}

export interface Progress {
  text: string;
  fraction: number | null;
}

export function progressView(event: WrapperProgress, size: (bytes: number) => string): Progress {
  const known = event.total > 0;
  const fraction = known ? Math.min(1, Math.max(0, event.done / event.total)) : null;
  if (event.stage === 'download') {
    const text = known
      ? msg('modals.compatDownloading', { done: size(event.done), total: size(event.total) })
      : msg('modals.compatDownloadingUnknown', { done: size(event.done) });
    return { text, fraction };
  }
  const extract = event.stage === 'extract';
  if (!known) return { text: msg(extract ? 'modals.compatExtractingUnknown' : 'modals.compatApplyingUnknown'), fraction };
  const key = extract ? 'modals.compatExtracting' : 'modals.compatApplying';
  return { text: msg(key, { done: event.done, total: event.total }), fraction };
}

export function problemText(code: string, file?: string): string {
  const short = code.replace(/^wrappers\./, '');
  if (short === 'tampered' && file) return msg('modals.compatProblemFile', { file });
  const key = `wrappers.${short}`;
  return hasMessage(key) ? msg(key) : msg('modals.compatProblemUnknown');
}
