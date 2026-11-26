import { afterEach, describe, expect, it, vi } from 'vitest';
import { setTags } from '../services/library';
import { report } from '../services/reviews';
import { calls, resetBindings } from './fakeBinding';
import { bindingId, sentToGo } from './gameDialogsTesting';

vi.mock('@wailsio/runtime', async () => (await import('./gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));

afterEach(() => {
  resetBindings();
});

describe('the recording runtime of the game dialogs\' tests', () => {
  it('names a binding by the id the generated code calls Go with', () => {
    expect(bindingId('metadata.SetLanguage')).toBe(1365384914);
    expect(bindingId('library.PlayGame')).toBe(55624214);
  });

  it('records a call a real service makes, instead of sending it', async () => {
    await setTags('g1', ['coop']);
    await report(42, 'spam');

    expect(sentToGo().map((call) => call.key)).toEqual([`Call.ByID:${bindingId('library.SetTags')}`, `Call.ByID:${bindingId('reviews.Report')}`]);
    expect(calls[0].args).toEqual(['g1', ['coop']]);
  });

  it('leaves out only the interface language the metadata service tells Go', () => {
    calls.push({ key: `Call.ByID:${bindingId('metadata.SetLanguage')}`, args: ['ru'] }, { key: `Call.ByID:${bindingId('library.PlayGame')}`, args: ['g1'] });

    expect(sentToGo().map((call) => call.key)).toEqual([`Call.ByID:${bindingId('library.PlayGame')}`]);
  });
});
