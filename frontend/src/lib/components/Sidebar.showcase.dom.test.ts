import { screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// Unset, the gate is the real one, read in this run's mode ('test'); set, it stands for a build of
// that kind. showcase.test.ts proves the gate follows the mode.
const gate = vi.hoisted(() => ({ open: undefined as boolean | undefined }));
vi.mock('../navigation/showcase', async (importOriginal) => {
  const real = await importOriginal<typeof import('../navigation/showcase')>();
  return {
    ...real,
    get SHOWCASE() {
      return gate.open ?? real.SHOWCASE;
    },
  };
});

import { resetHistory, route } from '../stores/router';
import { renderDom } from '../testing/dom';
import Sidebar from './Sidebar.svelte';

const entry = () => screen.queryByRole('button', { name: 'Component showcase' });

afterEach(() => {
  gate.open = undefined;
  resetHistory();
});

describe('the showcase entry in the sidebar', () => {
  it('is absent from a build that is not a development one', () => {
    renderDom(Sidebar);
    expect(entry()).toBeNull();
  });

  it('is absent while the gate is shut', () => {
    gate.open = false;
    renderDom(Sidebar);
    expect(entry()).toBeNull();
    expect(screen.getAllByRole('button').length).toBeGreaterThan(0);
  });

  it('in a development build opens the showcase and says it is the page shown', async () => {
    gate.open = true;
    const { user } = renderDom(Sidebar);
    const button = entry();
    expect(button).not.toBeNull();

    await user.click(button as HTMLElement);
    await tick();

    expect(get(route).name).toBe('showcase');
    expect(button?.getAttribute('aria-current')).toBe('page');
  });
});
