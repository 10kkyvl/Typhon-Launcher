import { describe, expect, it } from 'vitest';
import app from '../../App.svelte?raw';
import main from '../../main.ts?raw';
import html from '../../../index.html?raw';
import overlay from '../components/UpdateOverlay.svelte?raw';

// A stylesheet imported as text comes back empty under vitest, and Go sources are outside Vite's root.
type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const tokens = fs.readFileSync(new URL('../../styles/tokens.css', import.meta.url), 'utf8');
const mainGo = fs.readFileSync(new URL('../../../../main.go', import.meta.url), 'utf8');

// Tripwires over the shared files that wire a successor of a seamless restart: the parts that only a
// running Wails window exercises. They do not prove the behaviour; the live run does.

describe('the wiring of a successor', () => {
  it('reads the hand-over before the app mounts, and paints its colour before any stylesheet', () => {
    const read = main.indexOf('loadHandoff()');
    const mountAt = main.indexOf('mount(App');
    expect(read).toBeGreaterThan(-1);
    expect(read).toBeLessThan(mountAt);
    const boot = html.indexOf('id="boot-bg"');
    expect(boot).toBeGreaterThan(-1);
    expect(boot).toBeLessThan(html.indexOf('</head>'));
    // The dark theme's --bg, in the page before any stylesheet and in the window Go opens: another
    // colour there is a flash at every start.
    const bg = /:root\s*\{[^}]*?--bg:\s*(#[0-9a-f]{6});/.exec(tokens)?.[1] ?? '';
    expect(bg).not.toBe('');
    expect(html).toContain(`background: ${bg};`);
    const [r, g, b] = [1, 3, 5].map((at) => parseInt(bg.slice(at, at + 2), 16));
    expect(mainGo).toContain(`windowBackground := application.NewRGB(${r}, ${g}, ${b})`);
  });

  it('leaves the in-game overlay window transparent from its first frame', () => {
    const drop = html.indexOf("has('overlay')) document.getElementById('boot-bg')?.remove()");
    expect(drop).toBeGreaterThan(html.indexOf('id="boot-bg"'));
    expect(drop).toBeLessThan(html.indexOf('</head>'));
  });

  it('mounts the show once, over every screen, from the first frame in a successor', () => {
    expect(app.match(/<UpdateShow \/>/g)).toHaveLength(1);
    expect(app).toMatch(/\{#if shellDecided \|\| \$updateShow\?\.held\}\s*\{#key \$locale\}<UpdateShow \/>\{\/key\}\s*\{\/if\}/);
    expect(overlay).not.toContain('<UpdateShow');
    expect(app).toMatch(/\$effect\(\(\) => setShellReady\(shellDecided\)\)/);
  });
});
