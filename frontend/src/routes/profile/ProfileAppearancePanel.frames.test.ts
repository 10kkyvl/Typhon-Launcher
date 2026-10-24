import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
import Panel from './ProfileAppearancePanel.svelte';
import raw from './ProfileAppearancePanel.svelte?raw';
import { currentUser } from '../../lib/stores/user';
import { DEFAULT_PROFILE, AccountError, type CurrentUser } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { locale } from '../../lib/i18n/locale';

const id = '3f2b8c1e-9a4d-4e57-b0aa-1c2d3e4f5a6b';
const frames = [{ id, name: 'Dachshund <b>', url: 'https://cdn/dog.png', hole: 0.5 }];

function user(extra: Partial<CurrentUser>): CurrentUser {
  return { id: 'u1', username: 'ann', displayName: 'Ann', email: '', avatarUrl: '', bio: '', profile: DEFAULT_PROFILE, createdAt: '', ...extra };
}

function html(avatarFrame: string, owned: CurrentUser['frames']) {
  currentUser.set(user({ frames: owned }));
  const draft = { ...DEFAULT_PROFILE, appearance: { ...(DEFAULT_PROFILE.appearance ?? {}), avatarFrame } };
  return render(Panel, { props: { draft, disabled: false } } as never).body;
}

describe('personal frames in the appearance panel', () => {
  it('lists granted frames as plain text and hides the frame colour when one is worn', () => {
    const out = html(`custom:${id}`, frames);
    expect(out).toContain('Персональные');
    expect(out).toContain('src="https://cdn/dog.png"');
    expect(out).toContain('Dachshund &lt;b>');
    expect(out).not.toContain('Dachshund <b>');
    expect(out).not.toContain('Рамка отозвана');
    expect(out).not.toContain('Цвет рамки');
  });
  it('shows the frame colour for presets', () => {
    expect(html('ring', frames)).toContain('Цвет рамки');
  });
  it('has no personal section without granted frames', () => {
    expect(html('none', null)).not.toContain('Персональные');
    expect(html('none', [])).not.toContain('Персональные');
  });
  it('says the frame was revoked and keeps the colour field', () => {
    const out = html(`custom:${id}`, []);
    expect(out).toContain('Рамка отозвана');
    expect(out).not.toContain('Персональные');
    expect(out).toContain('Цвет рамки');
  });
  it('picks an owned frame by writing custom:<id> into the appearance', () => {
    const source = raw.replace(/\r\n/g, '\n');
    expect(source).toContain('onclick={() => patch({ avatarFrame: `custom:${owned.id}` })}');
  });
});

describe('frame_not_granted', () => {
  it('has its own text in both languages', () => {
    const err = new AccountError('frame_not_granted');
    expect(err.field).toBe('profile');
    locale.set('ru');
    expect(accountErrorText(err)).toBe('Эта рамка больше не выдана');
    locale.set('en');
    expect(accountErrorText(err)).not.toBe('Эта рамка больше не выдана');
    expect(accountErrorText(err)).toMatch(/frame/i);
    locale.set('ru');
  });
});
