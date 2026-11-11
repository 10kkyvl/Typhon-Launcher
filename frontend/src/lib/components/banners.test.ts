import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/backend', () => ({ inWails: false }));

import NetworkBanner from './NetworkBanner.svelte';
import OfflineBanner from './OfflineBanner.svelte';
import { msg } from '../i18n';
import { networkState } from '../stores/network';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const source = (file: string) => fs.readFileSync(new URL(file, import.meta.url), 'utf8');
const rule = (text: string, selector: string) => new RegExp(`\\n {2}\\${selector} \\{([^}]*)\\}`).exec(text)?.[1] ?? '';

// The offline and network banners are rows of the content column under the top bar. Each is a notice:
// an opaque plate in its tone, so the row reads over the window backdrop.
describe('status banners', () => {
  for (const [file, selector] of [
    ['./OfflineBanner.svelte', '.offline-banner'],
    ['./NetworkBanner.svelte', '.network-banner'],
  ] as const) {
    it(`${file} is a row inset to the page margins with no fill of its own`, () => {
      const body = rule(source(file), selector);
      expect(body).toContain('margin: 0 var(--page-x) var(--space-3)');
      expect(body).toContain('flex-shrink: 0');
      expect(body).not.toMatch(/background|border/);
      expect(source(file)).not.toMatch(/-subtle/);
    });
  }

  it('the offline banner is a warning notice and the network banner a failure notice', () => {
    expect(render(OfflineBanner, { props: {} } as never).body).toContain('data-tone="warning"');
    networkState.set({ mode: 'proxy', state: 'down', code: 'download.proxy_unreachable', reason: '', address: '', warning: '' });
    const network = render(NetworkBanner).body;
    networkState.set(null);
    expect(network).toContain('data-tone="danger"');
    expect(network).toContain('role="alert"');
  });

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
