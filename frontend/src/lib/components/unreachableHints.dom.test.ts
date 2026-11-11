import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { statusLabel } from '../game/status';
import { msg } from '../i18n';
import { authState, currentUser } from '../stores/user';
import { description, hintOnFocus, tabbable } from '../testing/hints';
import { user } from '../testing/dom';
import AccountMenu from './AccountMenu.svelte';
import Button from './Button.svelte';
import CollectionBlock from './profile/CollectionBlock.svelte';
import StatusBadge from './StatusBadge.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// Hints that sat on something no keyboard reaches (a span around a disabled button, a badge, a mark
// inside a control) and carried meaning: each now reaches a keyboard and a screen-reader user.

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  currentUser.set(null);
  authState.set('bootstrapping');
});

const text = (value: string) => createRawSnippet(() => ({ render: () => `<span>${value}</span>` }));

describe('hints no keyboard reached', () => {
  it('a disabled button with a reason stays in the Tab order, ignores presses and is described by the reason', async () => {
    const onclick = vi.fn();
    render(Button, { target: app, props: { disabled: true, reason: 'The game is running', onclick, children: text('Move') } });
    const button = screen.getByRole('button', { name: 'Move' });
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(tabbable(button)).toBe(true);
    expect(description(button)).toBe('The game is running');
    expect(await hintOnFocus(button)).toBe('The game is running');
    await user().click(button);
    expect(onclick).not.toHaveBeenCalled();
  });

  it('a disabled button without a reason stays a plain disabled button', () => {
    render(Button, { target: app, props: { disabled: true, children: text('Move') } });
    const button = screen.getByRole('button', { name: 'Move' }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect(button.hasAttribute('aria-describedby')).toBe(false);
  });

  it('a status badge with a hint can be reached and is described by it; one without stays out of the Tab order', async () => {
    render(StatusBadge, { target: app, props: { kind: 'neutral', label: 'link', hint: 'Size of the folder the link points to' } });
    const badge = screen.getByText('link').closest('.badge') as HTMLElement;
    expect(tabbable(badge)).toBe(true);
    expect(description(badge)).toBe('Size of the folder the link points to');
    expect(await hintOnFocus(badge)).toBe('Size of the folder the link points to');

    render(StatusBadge, { target: app, props: { kind: 'neutral', label: 'external' } });
    expect((screen.getByText('external').closest('.badge') as HTMLElement).hasAttribute('tabindex')).toBe(false);
  });

  it('the account chip says the profile is a cached copy while offline', async () => {
    authState.set('offline');
    currentUser.set({ id: 'me', username: 'me', displayName: 'Me', avatarUrl: '' } as never);
    render(AccountMenu, { target: app });
    const chip = screen.getByRole('button', { name: /Me/ });
    expect(description(chip)).toBe(msg('ui.offlineProfileTooltip'));
    expect(await hintOnFocus(chip)).toBe(msg('ui.offlineProfileTooltip'));
  });

  it('a mastered game in a collection says what its 100% means', async () => {
    const games = [{ key: 'g1', title: 'Hades', cover: '', hero: '', status: 'mastered', open: () => {} }];
    render(CollectionBlock, { target: app, props: { title: 'Completed', games, dated: true } });
    const tile = screen.getByRole('button', { name: /Hades/ });
    expect(description(tile)).toBe(statusLabel('mastered'));
    expect(await hintOnFocus(tile)).toBe(statusLabel('mastered'));
  });
});
