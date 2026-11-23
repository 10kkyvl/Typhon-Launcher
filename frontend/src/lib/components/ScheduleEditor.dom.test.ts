import { afterEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import ScheduleEditor from './ScheduleEditor.svelte';

const NIGHTS = [{ days: [1, 2], from: '01:00', to: '07:00' }];

afterEach(() => {
  document.body.replaceChildren();
});

function words() {
  const label = document.createElement('span');
  label.id = 'schedule-label';
  label.textContent = 'Schedule';
  const hint = document.createElement('p');
  hint.id = 'schedule-hint';
  hint.textContent = 'Outside the windows, downloads stand still';
  document.body.append(label, hint);
}

const controls = (root: HTMLElement) => [...root.querySelectorAll<HTMLButtonElement | HTMLInputElement>('button, input')];

describe('ScheduleEditor', () => {
  it('adds a window and reports the whole schedule', async () => {
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(ScheduleEditor, { windows: [], onchange });
    await user.click(getByRole('button', { name: msg('settings.downloadsScheduleAdd') }));
    expect(onchange).toHaveBeenCalledExactlyOnceWith([{ days: [0, 1, 2, 3, 4, 5, 6], from: '01:00', to: '07:00' }]);
  });

  it('without a name, a description or disabled, is a plain block with every control live', () => {
    const { container } = renderDom(ScheduleEditor, { windows: NIGHTS, onchange: vi.fn() });
    const root = container.querySelector('.schedule') as HTMLElement;
    expect(root.hasAttribute('role')).toBe(false);
    expect(root.hasAttribute('aria-labelledby')).toBe(false);
    expect(root.hasAttribute('aria-describedby')).toBe(false);
    expect(controls(root).length).toBeGreaterThan(0);
    expect(controls(root).filter((control) => control.disabled)).toEqual([]);
  });

  it('is a group named and described by the words its caller names', () => {
    words();
    const { getByRole } = renderDom(ScheduleEditor, { windows: NIGHTS, labelledby: 'schedule-label', describedby: 'schedule-hint', onchange: vi.fn() });
    const group = getByRole('group', { name: 'Schedule' });
    expect(description(group)).toBe('Outside the windows, downloads stand still');
  });

  it('turns every control off, the windows still shown, and reports nothing', async () => {
    words();
    const onchange = vi.fn();
    const { getByRole, user } = renderDom(ScheduleEditor, { windows: NIGHTS, labelledby: 'schedule-label', disabled: true, onchange });
    const group = getByRole('group', { name: 'Schedule' });
    expect(controls(group).filter((control) => !control.disabled)).toEqual([]);
    expect((group.querySelector('input[type="time"]') as HTMLInputElement).value).toBe('01:00');

    await user.click(getByRole('button', { name: msg('settings.downloadsScheduleAdd') }));
    expect(onchange).not.toHaveBeenCalled();
  });
});
