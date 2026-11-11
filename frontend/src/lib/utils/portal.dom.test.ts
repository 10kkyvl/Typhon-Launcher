import { afterEach, describe, expect, it } from 'vitest';
import { portal } from './portal';

afterEach(() => {
  document.body.replaceChildren();
});

describe('portal in a DOM', () => {
  it('moves the node out of a click-through container to the end of the body and removes it on teardown', () => {
    const container = document.createElement('div');
    container.style.pointerEvents = 'none';
    const node = document.createElement('div');
    container.append(node);
    document.body.append(container, document.createElement('aside'));

    const teardown = portal(node);

    expect(node.parentElement).toBe(document.body);
    expect(document.body.lastElementChild).toBe(node);
    expect(container.contains(node)).toBe(false);

    teardown?.();

    expect(node.isConnected).toBe(false);
  });
});
