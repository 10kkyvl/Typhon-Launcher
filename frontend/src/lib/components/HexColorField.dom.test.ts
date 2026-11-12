import { screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());

import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import HexColorField from './HexColorField.svelte';
import source from './HexColorField.svelte?raw';

describe('HexColorField', () => {
  it('is named by what colour it holds, with the visible "HEX" in the name', () => {
    renderDom(HexColorField, { label: 'Frame color', value: '#112233' });
    renderDom(HexColorField, { label: 'Name color', value: '#445566' });
    expect(screen.getByRole('textbox', { name: 'Frame color HEX' })).toBeTruthy();
    expect(screen.getByRole('textbox', { name: 'Name color HEX' })).toBeTruthy();
  });

  it('names each copy button for the colour it copies', () => {
    renderDom(HexColorField, { label: 'Frame color', value: '#112233' });
    renderDom(HexColorField, { label: 'Name color', value: '#445566' });
    expect(screen.getByRole('button', { name: msg('settings.copyHexOf', { name: 'Frame color' }) })).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('settings.copyHexOf', { name: 'Name color' }) })).toBeTruthy();
  });

  it('takes an id and the id of a message that describes it, so a page can tie its error to it', () => {
    const note = document.createElement('p');
    note.id = 'frame-invalid';
    note.textContent = 'Enter a complete color';
    document.body.append(note);
    renderDom(HexColorField, { label: 'Frame color', value: '#12', id: 'frame-hex', describedby: 'frame-invalid' });
    const field = screen.getByRole('textbox', { name: 'Frame color HEX' });
    expect(field.id).toBe('frame-hex');
    expect(field.getAttribute('aria-describedby')).toBe('frame-invalid');
    note.remove();
  });

  it('says it is invalid for a bad colour, and for an empty one only when a colour is required', async () => {
    const { rerender } = renderDom(HexColorField, { label: 'Frame color', value: '', optional: true });
    const field = screen.getByRole('textbox', { name: 'Frame color HEX' });
    expect(field.hasAttribute('aria-invalid')).toBe(false);
    await rerender({ label: 'Frame color', value: '#12', optional: true });
    expect(field.getAttribute('aria-invalid')).toBe('true');
    await rerender({ label: 'Frame color', value: '', optional: false });
    expect(field.getAttribute('aria-invalid')).toBe('true');
    await rerender({ label: 'Frame color', value: '#ABCDEF', optional: false });
    expect(field.hasAttribute('aria-invalid')).toBe(false);
  });

  it('is the shared text field and keeps no focus look of its own', () => {
    renderDom(HexColorField, { label: 'Frame color', value: '#112233' });
    expect(screen.getByRole('textbox', { name: 'Frame color HEX' }).classList.contains('input')).toBe(true);
    const style = /<style>([\s\S]*)<\/style>/.exec(source)?.[1] ?? '';
    expect(style).not.toMatch(/:focus/);
  });
});
