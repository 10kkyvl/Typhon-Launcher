import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

const TAGS = /<ShellBackdrop\b/g;

function markup(source: string): string {
  return source.replace(/<script[\s\S]*?<\/script>/g, '').replace(/<style[\s\S]*?<\/style>/g, '');
}

function mounts(tag: RegExp): string[] {
  return Object.entries(frontendSources).flatMap(([path, source]) => [...markup(source).matchAll(tag)].map(() => path));
}

describe('the window backdrop', () => {
  it('is mounted exactly once, in the main window shell', () => {
    expect(mounts(TAGS)).toEqual(['lib/components/AppShell.svelte']);
  });

  it('is the first thing in the shell, outside every condition, so all of the shell paints over it', () => {
    const shell = markup(frontendSources['lib/components/AppShell.svelte']);
    const open = shell.search(/<div class="shell"/);
    expect(open).toBeGreaterThan(-1);
    const inside = shell.slice(shell.indexOf('>', open) + 1);
    expect(inside.trimStart().startsWith('<ShellBackdrop />')).toBe(true);
  });

  it('comes only with the main window shell: never in Big Picture, the overlay or the screens before sign-in', () => {
    expect(mounts(/<AppShell\b/g)).toEqual(['App.svelte']);
    const elsewhere = [
      'lib/components/BigPictureShell.svelte',
      'lib/components/LegalAcceptanceScreen.svelte',
      'lib/components/TelemetryConsentScreen.svelte',
      'routes/auth/AuthScreen.svelte',
      ...Object.keys(frontendSources).filter((path) => path.startsWith('overlay/') || path.startsWith('lib/bigpicture/')),
    ];
    for (const path of elsewhere) {
      expect(frontendSources[path], path).toBeTypeOf('string');
      expect(frontendSources[path], path).not.toMatch(/ShellBackdrop|stores\/ambient|<AppShell\b/);
    }
  });

  it('blurs by downscaling, never with a CSS filter', () => {
    const files = Object.entries(frontendSources).filter(
      ([path]) =>
        path === 'lib/components/ShellBackdrop.svelte' ||
        path.startsWith('lib/backdrop/') ||
        path === 'lib/stores/ambient.ts' ||
        path === 'lib/utils/ambientHover.ts',
    );
    expect(files.map(([path]) => path)).toContain('lib/components/ShellBackdrop.svelte');
    for (const [path, source] of files) {
      expect(source, path).not.toMatch(/backdrop-filter|filter\s*:\s*[^;]*blur\(/);
    }
  });

  it('covers the whole window, under a title bar that paints no background of its own', () => {
    const layer = /\.backdrop\s*\{([^}]*)\}/.exec(frontendSources['lib/components/ShellBackdrop.svelte'])?.[1] ?? '';
    expect(layer).toMatch(/position:\s*fixed/);
    expect(layer).toMatch(/inset:\s*0;/);
    const bar = /\.titlebar\s*\{([^}]*)\}/.exec(frontendSources['lib/components/TitleBar.svelte'])?.[1];
    expect(bar).toBeTypeOf('string');
    expect(bar).not.toMatch(/background/);
  });

  // The look is read from the DOM (a probe element and a canvas), which must happen in an effect that the
  // theme-applied signal drives, never lazily in a derived that may run before the theme reaches the window.
  it('reads the theme from the window when it has been applied, in an effect, never in a derived', () => {
    const source = frontendSources['lib/components/ShellBackdrop.svelte'];
    expect(source).toMatch(/\$effect\(\(\) => \{\s*void \$themeApplied;[^}]*resolveColors\(/);
    for (const m of source.matchAll(/\$derived(?:\.by)?\(([\s\S]*?)\);\n/g)) {
      expect(m[1]).not.toMatch(/resolveColors|getComputedStyle|document\./);
    }
  });
});
