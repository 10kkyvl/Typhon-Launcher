import { describe, expect, it } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import settingsRowSource from './SettingsRow.svelte?raw';
import sliderSource from './Slider.svelte?raw';

/**
 * The stacked row's layout, read from the rules that make it: jsdom lays nothing out, so this is a
 * tripwire, and the showcase's stacked rows are what a browser measures (a switch at its 40 px, a
 * select at its 22rem, a button at its own width, a slider asking to fill at the text's measure).
 */

const row = styleOf(settingsRowSource, 'SettingsRow.svelte');
const slider = styleOf(sliderSource, 'Slider.svelte');

describe('a stacked settings row', () => {
  it('puts its controls under the text at their own width, at the start', () => {
    expect(ruleBody(row, '.control').display).toBe('flex');
    const control = ruleBody(row, '.stack .control');
    expect(control.display).toBeUndefined();
    expect(control['flex-direction']).toBe('column');
    expect(control['align-items']).toBe('flex-start');
    expect(control['flex-basis']).toBe('100%');
    expect(control['max-width']).toBe('var(--prose-max)');
    expect(control['grid-template-columns']).toBeUndefined();
  });

  it('lets a slider that asks to fill take the whole measure', () => {
    const fill = ruleBody(slider, '.slider.fills');
    expect(fill['align-self']).toBe('stretch');
    expect(fill.width).toBe('100%');
  });
});
