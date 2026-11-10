import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import OfflineBanner from './OfflineBanner.svelte';
import { msg } from '../i18n';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const source = (file: string) => fs.readFileSync(new URL(file, import.meta.url), 'utf8');
const rule = (text: string, selector: string) => new RegExp(`\\n {2}\\${selector} \\{([^}]*)\\}`).exec(text)?.[1] ?? '';

// The offline and network banners are rows of the content column under the top bar, drawn as rounded
// panels on their status colour like the islands above them.
describe('status banners', () => {
  for (const [file, selector, subtle] of [
    ['./OfflineBanner.svelte', '.offline-banner', '--warning-subtle'],
    ['./NetworkBanner.svelte', '.network-banner', '--danger-subtle'],
  ] as const) {
    it(`${file} is a rounded panel inset to the page margins`, () => {
      const body = rule(source(file), selector);
      expect(body).toContain(`background: var(${subtle})`);
      expect(body).toContain('border: 1px solid var(--border)');
      expect(body).toContain('border-radius: var(--radius-lg)');
      expect(body).toContain('margin: 0 var(--page-x) var(--space-3)');
      expect(body).not.toContain('border-bottom');
      expect(body).toContain('flex-shrink: 0');
    });
  }

  it('sit between the top bar and the page, inside the content column', () => {
    const shell = source('./AppShell.svelte');
    const main = shell.slice(shell.indexOf('<div class="main"'), shell.indexOf('</div>', shell.indexOf('<main')));
    const at = ['<Topbar />', '<OfflineBanner />', '<NetworkBanner />', '<main class="content"'].map((tag) => main.indexOf(tag));
    expect(at.every((i) => i >= 0)).toBe(true);
    expect([...at].sort((a, b) => a - b)).toEqual(at);
  });

  it('keeps the offline banner text and its three actions', () => {
    const body = render(OfflineBanner, { props: {} } as never).body;
    expect(body).toContain(msg('ui.offlineBannerText'));
    expect(body).toContain(msg('ui.offlineBannerStatus'));
    expect(body).toContain(msg('common.retry'));
    expect(body).toContain(`aria-label="${msg('common.close')}"`);
    expect(body).toContain('role="status"');
  });
});
