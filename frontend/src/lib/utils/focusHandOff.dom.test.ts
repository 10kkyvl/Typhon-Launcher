import { afterEach, describe, expect, it } from 'vitest';
import { focusHandOff } from './focusHandOff';

function page() {
  document.body.innerHTML = '<button id="pressed">Open</button><button id="next">Next</button><button id="other">Other</button>';
  const get = (id: string) => document.getElementById(id) as HTMLButtonElement;
  return { pressed: get('pressed'), next: get('next'), other: get('other') };
}

afterEach(() => {
  document.body.replaceChildren();
});

describe('focusHandOff', () => {
  it('moves focus to the target when the pressed control held focus and went away', async () => {
    const { pressed, next } = page();
    pressed.focus();
    const hand = focusHandOff();
    pressed.remove();

    await hand(() => next);

    expect(document.activeElement).toBe(next);
  });

  it('leaves focus alone after a press that held none', async () => {
    const { pressed, next } = page();
    const hand = focusHandOff();
    pressed.remove();

    await hand(() => next);

    expect(document.activeElement).toBe(document.body);
  });

  it('leaves focus on a control that is still there', async () => {
    const { pressed, next } = page();
    pressed.focus();
    const hand = focusHandOff();

    await hand(() => next);

    expect(document.activeElement).toBe(pressed);
  });

  it('leaves focus where the user has moved it meanwhile', async () => {
    const { pressed, next, other } = page();
    pressed.focus();
    const hand = focusHandOff();
    pressed.remove();
    other.focus();

    await hand(() => next);

    expect(document.activeElement).toBe(other);
  });

  it('counts focus inside what is on its way out as gone', async () => {
    document.body.innerHTML = '<div id="toast"><button id="pressed">Hide</button></div><button id="next">Next</button>';
    const pressed = document.getElementById('pressed') as HTMLButtonElement;
    const next = document.getElementById('next') as HTMLButtonElement;
    pressed.focus();
    const hand = focusHandOff(() => document.getElementById('toast'));

    await hand(() => next);

    expect(document.activeElement).toBe(next);
  });
});
