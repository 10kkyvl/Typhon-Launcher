import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import SettingsRowHarness from '../testing/SettingsRowHarness.svelte';
import SettingsRow from './SettingsRow.svelte';

describe('SettingsRow', () => {
  it('names each base control with its label and describes it with its hint', () => {
    const { getByRole } = renderDom(SettingsRowHarness);
    expect(getByRole('switch', { name: 'Запускать с системой', description: 'Лаунчер откроется при входе' })).toBeTruthy();
    expect(getByRole('combobox', { name: 'Язык', description: 'Язык интерфейса' })).toBeTruthy();
    expect(getByRole('slider', { name: 'Масштаб', description: 'Размер всего интерфейса' })).toBeTruthy();
  });

  it('names a single field through labelFor', () => {
    const { getByRole, getByText } = renderDom(SettingsRowHarness);
    expect(getByRole('textbox', { name: 'Потоки загрузки', description: 'Сколько частей качать сразу' })).toBeTruthy();
    expect(getByText('Потоки загрузки').tagName).toBe('LABEL');
  });

  it('reads the warning note with the hint and sets it apart', () => {
    const { getByRole, getByText } = renderDom(SettingsRowHarness);
    expect(getByRole('switch', { name: 'Анимации' }).getAttribute('aria-describedby')).toBeTruthy();
    expect(getByRole('switch', { name: 'Анимации', description: /Плавные переходы.*Система просит меньше движения/ })).toBeTruthy();
    expect(getByText('Система просит меньше движения').classList.contains('note')).toBe(true);
  });

  it('says why a row is disabled through the hint, and hands the state to its control', () => {
    const { getByRole } = renderDom(SettingsRowHarness, { reason: 'Недоступно в портативной версии' });
    const overlay = getByRole('switch', { name: 'Оверлей', description: /Недоступно в портативной версии/ });
    expect(overlay).toHaveProperty('disabled', true);
    expect(overlay.closest('.settings-row')?.getAttribute('data-disabled')).toBe('true');
  });

  it('leaves an enabled row and its control enabled', () => {
    const { getByRole } = renderDom(SettingsRowHarness);
    const overlay = getByRole('switch', { name: 'Оверлей', description: 'Панель поверх игры' });
    expect(overlay).toHaveProperty('disabled', false);
    expect(overlay.closest('.settings-row')?.hasAttribute('data-disabled')).toBe(false);
  });

  it('draws `below` full width under the row, with the ids', () => {
    const { getByRole } = renderDom(SettingsRowHarness);
    const list = getByRole('list', { name: 'Папки библиотеки' });
    expect(list.closest('.below')).toBeTruthy();
    expect(list.closest('.settings-row')?.querySelector('.control')).toBeNull();
  });

  it('puts the control under the text with `stack`', () => {
    const { getByRole } = renderDom(SettingsRowHarness);
    expect(getByRole('slider').closest('.settings-row')?.classList.contains('stack')).toBe(true);
    expect(getByRole('switch', { name: 'Запускать с системой' }).closest('.settings-row')?.classList.contains('stack')).toBe(false);
  });

  it('gives no hint id to a row without a hint', () => {
    const { container } = renderDom(SettingsRow, { label: 'Без подсказки' });
    expect(container.querySelector('.hint')).toBeNull();
  });

  it('repeats no id over two hundred rows', () => {
    const { container } = renderDom(SettingsRowHarness, { many: 200 });
    const ids = [...container.querySelectorAll('[id]')].map((el) => el.id);
    expect(ids.length).toBeGreaterThan(400);
    expect(ids.filter((id, index) => ids.indexOf(id) !== index)).toEqual([]);
  });
});
