import { readable } from 'svelte/store';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AppInfo } from '../services/system';

const handlers = new Map<string, () => void>();
let platform = 'windows';
let maximised = false;

vi.mock('@wailsio/runtime', () => ({
  Events: {
    Types: {
      Common: {},
      Windows: { WindowMaximise: 'windows:WindowMaximise', WindowRestore: 'windows:WindowRestore' },
      Mac: {},
    },
    On: vi.fn((name: string | undefined, cb: () => void) => {
      if (name) handlers.set(name, cb);
      return () => {};
    }),
  },
  Window: { IsMaximised: vi.fn(async () => maximised) },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/system', () => ({ appInfo: vi.fn(async () => ({ platform }) as AppInfo) }));
vi.mock('../stores/toasts', () => ({ toast: vi.fn() }));

async function bar() {
  vi.resetModules();
  handlers.clear();
  const store = await import('../stores/window');
  await store.initPlatform();
  store.initWindowState();
  const { default: TitleBar } = await import('./TitleBar.svelte');
  const { render } = await import('svelte/server');
  const { msg } = await import('../i18n');
  return { html: () => render(TitleBar).body, store, msg };
}

function band(html: string): string {
  const tag = /<div[^>]*class="titlebar[^"]*"[^>]*>/.exec(html)?.[0];
  expect(tag, 'title bar band').toBeDefined();
  return tag ?? '';
}

function controls(html: string): string {
  const start = html.indexOf('data-part="window-controls"');
  expect(start, 'window controls block').toBeGreaterThan(-1);
  return html.slice(start);
}

function buttons(html: string): string[] {
  return [...controls(html).matchAll(/<button[^>]*>/g)].map((match) => match[0]);
}

// The first import transforms the component and the i18n catalogs (seconds on a busy machine): that is
// paid here, not inside the first case's own time limit.
beforeAll(async () => {
  await bar();
}, 60_000);

beforeEach(() => {
  platform = 'windows';
  maximised = false;
});

describe('TitleBar', () => {
  it.each(['windows', 'linux', 'browser', 'darwin'])('draws the window buttons on %s', async (name) => {
    platform = name;
    const { html, msg } = await bar();
    const body = html();
    expect(band(body)).toContain('--wails-draggable: drag');
    const tags = buttons(body);
    expect(tags).toHaveLength(3);
    expect(tags.map((tag) => /aria-label="([^"]*)"/.exec(tag)?.[1])).toEqual([
      msg('ui.minimize'),
      msg('ui.maximize'),
      msg('common.close'),
    ]);
    for (const tag of tags) expect(tag).toMatch(/class="[^"]*\bno-drag\b/);
    for (const tag of tags) expect(tag).toContain('tabindex="-1"');
    expect(body).not.toContain('traffic-lights');
  });

  it('renders the controls exactly once', async () => {
    const { html } = await bar();
    expect(html().match(/data-part="window-controls"/g)).toHaveLength(1);
  });

  it('shows the restore glyph and label while the window is maximised', async () => {
    const { html, msg } = await bar();
    const normal = buttons(html())[1];
    expect(normal).toContain('data-maximised="false"');
    expect(normal).toContain(`aria-label="${msg('ui.maximize')}"`);

    handlers.get('windows:WindowMaximise')?.();
    const restored = buttons(html())[1];
    expect(restored).toContain('data-maximised="true"');
    expect(restored).toContain(`aria-label="${msg('ui.restore')}"`);
    expect(html()).toContain('lucide-copy');
    expect(html()).not.toContain('lucide-square');

    handlers.get('windows:WindowRestore')?.();
    expect(buttons(html())[1]).toContain('data-maximised="false"');
    expect(html()).toContain('lucide-square');
  });

  it('leaves room for the native traffic lights and draws no buttons in the native variant', async () => {
    vi.resetModules();
    vi.doMock('../stores/window', async (importOriginal) => ({
      ...(await importOriginal<object>()),
      windowControls: readable('native'),
    }));
    try {
      const { default: TitleBar } = await import('./TitleBar.svelte');
      const { render } = await import('svelte/server');
      const body = render(TitleBar).body;
      expect(band(body)).toContain('--wails-draggable: drag');
      expect(body).not.toContain('<button');
      expect(body).not.toContain('data-part="window-controls"');
      const spacer = /<div[^>]*class="traffic-lights\b[^"]*"[^>]*>/.exec(body)?.[0];
      expect(spacer, 'traffic lights spacer').toBeDefined();
      expect(spacer).not.toContain('no-drag');
    } finally {
      vi.doUnmock('../stores/window');
    }
  });
});
