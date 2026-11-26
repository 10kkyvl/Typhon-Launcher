import { describe, expect, it } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import shellSource from './BigPictureShell.svelte?raw';

/**
 * jsdom lays nothing out, so this is a tripwire on the rule: the footer's two-line controller hint
 * squeezed its gamepad icon to half its width.
 */

describe('the Big Picture footer', () => {
  it('keeps the controller hint icon at its own size when the hint wraps', () => {
    const shell = styleOf(shellSource, 'BigPictureShell.svelte');
    expect(ruleBody(shell, '.input-device :global(svg)').flex).toBe('none');
  });
});
