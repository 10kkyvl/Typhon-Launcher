import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/theme', async (original) => (await import('../testing/settingsDialogsTesting')).themeService(await original()));
vi.mock('../../../bindings/typhon/internal/theme', async (original) => (await import('../testing/settingsDialogsTesting')).fakeService(await original(), 'theme'));

import { msg } from '../i18n';
import { importThemeText, type Theme } from '../services/theme';
import { themeErrorText } from '../theme/themeErrors';
import { user } from '../testing/dom';
import { calls, resetBindings } from '../testing/fakeBinding';
import { deferred } from '../testing/settingsDialogsTesting';
import ThemeTextImportModal from './ThemeTextImportModal.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  vi.mocked(importThemeText).mockReset();
  expect(calls).toEqual([]);
  resetBindings();
});

const THEME = { id: 'neon', name: 'Neon' } as Theme;

function show(onimported: (theme: Theme) => void | Promise<void> = vi.fn()) {
  const rendered = render(ThemeTextImportModal, { target: app, props: { open: true, onimported } });
  const dialog = screen.getByRole('dialog', { name: msg('settings.appearanceImportTextTitle') });
  return {
    ...rendered,
    dialog,
    text: () => within(dialog).getByRole('textbox', { name: msg('settings.appearanceImportTextLabel') }) as HTMLTextAreaElement,
    name: () => within(dialog).getByRole('textbox', { name: msg('settings.appearanceImportTextNameLabel') }) as HTMLInputElement,
    save: () => within(dialog).getByRole('button', { name: msg('common.save') }),
    cancel: () => within(dialog).getByRole('button', { name: msg('common.cancel') }),
  };
}

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.replace(/\s+/g, ' ').trim())
    .join(' ');

describe('ThemeTextImportModal', () => {
  it('opens on the text field, its name the label, and the name field is described by its hint', () => {
    const { text, name } = show();
    expect(document.activeElement).toBe(text());
    expect(text().getAttribute('aria-invalid')).not.toBe('true');
    expect(describedBy(name())).toBe(msg('settings.appearanceImportTextNameHint'));
  });

  it('imports the pasted text under the trimmed name with one call, closes and hands the theme on', async () => {
    vi.mocked(importThemeText).mockResolvedValueOnce(THEME);
    const onimported = vi.fn();
    const { text, name, save } = show(onimported);
    const keys = user();
    await keys.click(text());
    await keys.paste('{"version":1}');
    await keys.type(name(), '  Neon  ');

    await keys.click(save());

    expect(importThemeText).toHaveBeenCalledExactlyOnceWith('{"version":1}', 'Neon');
    await vi.waitFor(() => expect(onimported).toHaveBeenCalledExactlyOnceWith(THEME));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('holds Save with its reason while no text is pasted, sending nothing', async () => {
    const { save } = show();
    const keys = user();
    expect(save().getAttribute('aria-disabled')).toBe('true');
    expect(save()).toHaveProperty('disabled', false);
    expect(describedBy(save())).toBe(msg('settings.appearanceImportTextEmpty'));

    await keys.click(save());

    expect(importThemeText).not.toHaveBeenCalled();
  });

  it('sends one import however often Save is pressed while it runs, Save busy and focused, the rest held', async () => {
    const importing = deferred<Theme>();
    vi.mocked(importThemeText).mockReturnValueOnce(importing.promise);
    const { dialog, text, name, save, cancel } = show();
    const keys = user();
    await keys.click(text());
    await keys.paste('{"version":1}');
    save().focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');
    await keys.click(save());

    expect(importThemeText).toHaveBeenCalledTimes(1);
    expect(save().getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(save());
    expect(text().readOnly).toBe(true);
    expect(name().readOnly).toBe(true);
    expect(text().disabled).toBe(false);
    expect(cancel().getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(cancel())).toBe(msg('settings.appearanceImportTextBusy'));
    await keys.keyboard('{Escape}');
    expect(dialog.isConnected).toBe(true);
    importing.resolve(THEME);
  });

  it('says a refused text at the text field, which takes the focus, and keeps what was pasted', async () => {
    const refused = new Error('--accent: typhon:theme.token_kind: colour wanted');
    vi.mocked(importThemeText).mockRejectedValueOnce(refused);
    const { dialog, text, name, save } = show();
    const keys = user();
    await keys.click(text());
    await keys.paste('{"version":1,"tokens":{"--accent":"12"}}');

    await keys.click(save());

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(themeErrorText(refused));
    expect(text().getAttribute('aria-invalid')).toBe('true');
    expect((text().getAttribute('aria-describedby') ?? '').split(' ')).toContain(alert.id);
    expect(name().getAttribute('aria-invalid')).not.toBe('true');
    expect(text().value).toBe('{"version":1,"tokens":{"--accent":"12"}}');
    expect(document.activeElement).toBe(text());
  });

  it('says a refused name at the name field when the player gave one', async () => {
    vi.mocked(importThemeText).mockRejectedValueOnce(new Error('typhon:theme.invalid_name: bad'));
    const { dialog, text, name, save } = show();
    const keys = user();
    await keys.click(text());
    await keys.paste('{"version":1}');
    await keys.type(name(), '###');

    await keys.click(save());

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('theme.invalid_name'));
    expect(name().getAttribute('aria-invalid')).toBe('true');
    expect((name().getAttribute('aria-describedby') ?? '').split(' ')).toContain(alert.id);
    expect(text().getAttribute('aria-invalid')).not.toBe('true');
    expect(document.activeElement).toBe(name());
    expect(name().value).toBe('###');
  });

  it('says a refused name at the text field when the name came from the text', async () => {
    vi.mocked(importThemeText).mockRejectedValueOnce(new Error('typhon:theme.invalid_name: bad'));
    const { dialog, text, name, save } = show();
    const keys = user();
    await keys.click(text());
    await keys.paste('{"version":1}');

    await keys.click(save());

    await within(dialog).findByRole('alert');
    expect(text().getAttribute('aria-invalid')).toBe('true');
    expect(name().getAttribute('aria-invalid')).not.toBe('true');
  });

  it('says a failure that is not about the text in the dialog by the catalogue, never in Go’s words', async () => {
    vi.mocked(importThemeText).mockRejectedValueOnce(new Error('write /themes/neon.json: no space left on device'));
    const { dialog, text, save } = show();
    const keys = user();
    await keys.click(text());
    await keys.paste('{"version":1}');

    await keys.click(save());

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('errLibrary.themeErrFallback'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(dialog.textContent).not.toContain('no space left');
    expect(text().getAttribute('aria-invalid')).not.toBe('true');
    expect(describedBy(save())).toBe(msg('errLibrary.themeErrFallback'));
    expect(document.activeElement).toBe(save());
  });

  it('takes the error away from the field once the player edits it', async () => {
    vi.mocked(importThemeText).mockRejectedValueOnce(new Error('typhon:theme.invalid_format: not json'));
    const { dialog, text, save } = show();
    const keys = user();
    await keys.click(text());
    await keys.paste('nope');
    await keys.click(save());
    await within(dialog).findByRole('alert');

    await keys.type(text(), '!');

    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(text().getAttribute('aria-invalid')).not.toBe('true');
  });

  it('starts every opening empty, with no error', async () => {
    vi.mocked(importThemeText).mockRejectedValueOnce(new Error('typhon:theme.invalid_format: not json'));
    const { dialog, rerender, text, save } = show();
    const keys = user();
    await keys.click(text());
    await keys.paste('nope');
    await keys.click(save());
    await within(dialog).findByRole('alert');

    await rerender({ open: false });
    await rerender({ open: true });

    const again = screen.getByRole('dialog', { name: msg('settings.appearanceImportTextTitle') });
    expect(within(again).queryByRole('alert')).toBeNull();
    expect((within(again).getByRole('textbox', { name: msg('settings.appearanceImportTextLabel') }) as HTMLTextAreaElement).value).toBe('');
  });
});
