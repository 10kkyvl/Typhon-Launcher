import { describe, expect, it } from 'vitest';
import { ruleBody } from '../testing/css';

// Read from disk: vitest serves `.css?raw` as an empty string.
type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const styles = fs.readFileSync(new URL('./styles.css', import.meta.url), 'utf8');

/**
 * jsdom lays nothing out, so this is a tripwire on the rule: a Big Picture button whose label wraps
 * (the menu's "back to the desktop mode") squeezed its icon to two thirds of its width.
 */

describe('a Big Picture button', () => {
  it('keeps its icon at its own size when the label wraps', () => {
    const icon = ruleBody(styles.replace(/\/\*[\s\S]*?\*\//g, ''), '.big-picture .bp-button > svg');
    expect(icon.flex).toBe('none');
  });
});
