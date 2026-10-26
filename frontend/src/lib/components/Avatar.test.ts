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

  it('leaves the accent in charge when the frame colour is empty', () => {
    expect(html({ frame: 'ring', frameColor: '' })).not.toContain('--frame-color');
  });
});

describe('Avatar personal frame', () => {
  const frame = 'custom:3f2b8c1e-9a4d-4e57-b0aa-1c2d3e4f5a6b';
  const frameImage = { url: 'https://cdn/dog.png', hole: 0.5 };

  it('lays the image over the avatar at the size the hole asks for', () => {
    const out = html({ frame, frameImage });
    expect(out).toContain('class="frame-art svelte-');
    expect(out).toContain('src="https://cdn/dog.png"');
    expect(out).toContain('width: calc(var(--size) * 2)');
    expect(out).toContain('alt=""');
    expect(out).toContain('aria-hidden="true"');
    expect(out).toContain('decoding="async"');
    expect(out).toContain('loading="lazy"');
    expect(out).toContain('framed');
  });
  it('centres the image when the hole is centred or the centre is absent', () => {
    expect(html({ frame, frameImage })).toContain('transform: translate(-50%, -50%)');
    expect(html({ frame, frameImage: { ...frameImage, holeX: 0.5, holeY: 0.5 } })).toContain('transform: translate(-50%, -50%)');
  });
  it('shifts the image so the offset hole lands on the avatar centre', () => {
    const out = html({ frame, frameImage: { ...frameImage, holeX: 0.4, holeY: 0.62 } });
    expect(out).toContain('width: calc(var(--size) * 2)');
    expect(out).toContain('transform: translate(-40%, -62%)');
  });
  it('reserves the overhang as margin and clips the image to the bleed box', () => {
    const out = html({ frame, frameImage: { ...frameImage, holeX: 0.9, holeY: 0.1 } });
    expect(out).toContain('--frame-bleed: 0.5');
    expect(out).toContain('--bleed-l: 0.5');
    expect(out).toContain('--bleed-r: 0');
    expect(out).toContain('--bleed-t: 0');
    expect(out).toContain('--bleed-b: 0.5');
    expect(out).toContain('class="frame-clip svelte-');
  });
  it('reserves nothing for an unframed avatar or a preset', () => {
    expect(html({})).not.toContain('--bleed');
    expect(html({ frame: 'ring' })).not.toContain('--bleed');
  });
  it('draws nothing for a custom frame without an image', () => {
    const out = html({ frame });
    expect(out).not.toContain('frame-art');
    expect(out).not.toContain('framed');
  });
  it('ignores the image for a preset, none or a malformed id', () => {
    expect(html({ frame: 'none', frameImage })).not.toContain('frame-art');
    expect(html({ frame: 'ring', frameImage })).not.toContain('frame-art');
    expect(html({ frame: 'custom:bad', frameImage })).not.toContain('frame-art');
  });
});
