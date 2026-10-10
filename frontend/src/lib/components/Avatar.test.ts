import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import Avatar from './Avatar.svelte';
import { AVATAR_FRAMES } from '../services/account';

function html(props: Record<string, unknown>) {
  return render(Avatar, { props: { name: 'Ann', ...props } } as never).body;
}

describe('Avatar frame', () => {
  it('renders no frame element by default or for none', () => {
    expect(html({})).not.toContain('frame');
    expect(html({ frame: 'none' })).not.toContain('frame');
  });

  it.each(AVATAR_FRAMES.filter((frame) => frame !== 'none'))('renders the %s frame', (frame) => {
    const out = html({ frame });
    expect(out).toContain(`frame-${frame}`);
    expect(out).toContain('framed');
    expect(out).toContain('aria-hidden="true"');
  });

  it('takes the frame colour from the prop', () => {
    expect(html({ frame: 'ring', frameColor: '#ff8800' })).toContain('--frame-color: #ff8800');
  });
});
