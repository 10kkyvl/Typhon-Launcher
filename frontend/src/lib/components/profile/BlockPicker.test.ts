import { render } from 'svelte/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../../services/backend', () => ({ inWails: false }));

import BlockPicker from './BlockPicker.svelte';
import BlockSettings from './BlockSettings.svelte';
import { locale } from '../../i18n';
import type { ProfileBlock, ProfileLayout } from '../../services/account';

function block(id: string, type: string, config: Record<string, unknown> = {}): ProfileBlock {
  return { id, type, width: 'full', config };
}

function layoutOf(count: number, unknown: number): ProfileLayout {
  const known = Array.from({ length: count - unknown }, (_, i) => block(`p${i}`, 'pinned', { igdbId: i + 1, caption: '' }));
  const later = Array.from({ length: unknown }, (_, i) => block(`u${i}`, 'future'));
  return { version: 1, blocks: [...known, ...later] };
}

function picker(layout: ProfileLayout): string {
  return render(BlockPicker, { props: { layout, onadd: () => {} } }).body;
}

beforeEach(() => {
  locale.set('en');
});

afterEach(() => {
  locale.set('ru');
});

describe('BlockPicker', () => {
  it('explains that blocks from a newer launcher fill the limit', () => {
    const out = picker(layoutOf(16, 3));

    expect(out).toContain('newer version of the launcher');
    expect(out).toContain('3 of 16');
  });

  it('stays quiet while there is room, or when every block is known', () => {
    expect(picker(layoutOf(15, 3))).not.toContain('newer version');
    expect(picker(layoutOf(16, 0))).not.toContain('newer version');
  });
});

describe('BlockSettings field errors', () => {
  const props = {
    layout: { version: 1, blocks: [] } as ProfileLayout,
    titleOf: () => '',
    onconfig: () => {},
    onremember: () => {},
    onremove: () => {},
    onclose: () => {},
  };

  it('puts the character message under the body of a text block', () => {
    const out = render(BlockSettings, {
      props: { ...props, block: block('t', 'text', { title: '', body: 'a\tb' }), issue: { code: 'bad_chars', field: 'body' } },
    }).body;

    expect(out).toContain('characters that are not allowed');
  });

  it('shows no message for an unrelated issue or a field that is fine', () => {
    const text = block('t', 'text', { title: '', body: '' });

    expect(render(BlockSettings, { props: { ...props, block: text, issue: { code: 'incomplete', field: 'body' } } }).body).not.toContain('not allowed');
    expect(render(BlockSettings, { props: { ...props, block: text, issue: null } }).body).not.toContain('not allowed');
  });
});
