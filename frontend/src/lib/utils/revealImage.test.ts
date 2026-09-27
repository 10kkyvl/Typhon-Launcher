import { describe, expect, it } from 'vitest';
import { revealImage } from './revealImage';

function image(complete = false, naturalWidth = 0) {
  return Object.assign(new EventTarget(), { complete, naturalWidth, dataset: {} }) as unknown as HTMLImageElement;
}

describe('revealImage', () => {
  it('keeps a pending image hidden until a successful load', () => {
    const node = image();
    revealImage(node);
    expect(node.dataset.ready).toBeUndefined();
    Object.assign(node, { naturalWidth: 300 });
    node.dispatchEvent(new Event('load'));
    expect(node.dataset.ready).toBe('true');
  });

  it('reveals an already cached image', () => {
    const node = image(true, 300);
    revealImage(node);
    expect(node.dataset.ready).toBe('true');
  });

  it('does not reveal a failed image or update a removed one', () => {
    const node = image(true);
    const action = revealImage(node);
    expect(node.dataset.ready).toBeUndefined();
    action.destroy();
    Object.assign(node, { naturalWidth: 300 });
    node.dispatchEvent(new Event('load'));
    expect(node.dataset.ready).toBeUndefined();
  });
});
