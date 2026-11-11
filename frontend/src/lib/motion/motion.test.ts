import { describe, expect, it } from 'vitest';
import { motionAllowed } from './motion';

const root = (classes: string[] = []) => ({ classList: { contains: (name: string) => classes.includes(name) } });

describe('motionAllowed', () => {
  it('follows the animations setting and the system reduced-motion request', () => {
    expect(motionAllowed(root(), false)).toBe(true);
    expect(motionAllowed(root(['no-anim']), false)).toBe(false);
    expect(motionAllowed(root(), true)).toBe(false);
  });
});
