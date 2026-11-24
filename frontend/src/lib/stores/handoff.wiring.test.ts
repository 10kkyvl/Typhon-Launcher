import { describe, expect, it } from 'vitest';
import app from '../../App.svelte?raw';
import main from '../../main.ts?raw';
import html from '../../../index.html?raw';
import overlay from '../components/UpdateOverlay.svelte?raw';

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
    // Go's defaultHandoffBackground and the dark theme's --bg.
    expect(html).toContain('#090d17');
  });

  it('mounts the show once, over every screen, from the first frame in a successor', () => {
    expect(app.match(/<UpdateShow \/>/g)).toHaveLength(1);
    expect(app).toMatch(/\{#if shellDecided \|\| \$updateShow\?\.held\}\s*\{#key \$locale\}<UpdateShow \/>\{\/key\}\s*\{\/if\}/);
    expect(overlay).not.toContain('<UpdateShow');
    expect(app).toMatch(/\$effect\(\(\) => setShellReady\(shellDecided\)\)/);
  });
});
