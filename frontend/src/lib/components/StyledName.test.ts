import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';
vi.mock('../services/backend', () => ({ inWails: false }));
import StyledName from './StyledName.svelte';

function html(props: Record<string, unknown>) {
  return render(StyledName, { props } as never).body;
}

describe('StyledName', () => {
  it('renders without a custom colour by default', () => {
    const out = html({ name: 'Ann', styleName: 'gradient' });
    expect(out).toContain('Ann');
    expect(out).not.toContain('--name-color');
    expect(out).not.toContain('custom');
  });
  it('passes a valid colour and its gradient end to the style', () => {
    const out = html({ name: 'Ann', styleName: 'gradient', nameColor: '#401020' });
    expect(out).toContain('--name-color: #401020');
    expect(out).toMatch(/--name-end: #[0-9a-f]{6}/);
    expect(out).toContain('custom');
  });
  it('applies the colour to the plain style too', () => {
    expect(html({ name: 'Ann', nameColor: '#ff8800' })).toContain('--name-color: #ff8800');
  });
  it('ignores a colour that is not a full hex value', () => {
    for (const bad of ['red', '#12345', '#112233;display:none', '']) {
      expect(html({ name: 'Ann', styleName: 'glow', nameColor: bad })).not.toContain('--name-color');
    }
  });
});
