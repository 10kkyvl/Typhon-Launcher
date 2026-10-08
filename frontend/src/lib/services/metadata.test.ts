import { beforeEach, describe, expect, it, vi } from 'vitest';
import { answer, calls, resetBindings } from '../testing/fakeBinding';

const FAKE = '../testing/fakeBinding';

vi.mock('../../../bindings/typhon/internal/metadata', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('metadata'),
}));

type Fn = (...args: unknown[]) => Promise<unknown>;

async function load(inWails: boolean) {
  vi.resetModules();
  vi.doMock('./backend', () => ({ inWails }));
  const metadata = (await import('./metadata')) as unknown as Record<string, Fn>;
  const { locale } = await import('../i18n/locale');
  return { metadata, locale };
}

function keys(): string[] {
  return calls.map((call) => call.key);
}

beforeEach(() => {
  resetBindings();
});

describe('metadata language', () => {
  it('tells the backend the interface language before it asks for anything', async () => {
    answer('metadata.GetView', { game: { id: 'g1' } });
    const { metadata, locale } = await load(true);
    locale.set('en');
    resetBindings();
    answer('metadata.GetView', { game: { id: 'g1' } });

    await metadata.getMetadataView('g1');

    const order = keys();
    expect(order.indexOf('metadata.SetLanguage')).toBeGreaterThan(-1);
    expect(order.indexOf('metadata.SetLanguage')).toBeLessThan(order.indexOf('metadata.GetView'));
    const language = calls.filter((call) => call.key === 'metadata.SetLanguage').at(-1);
    expect(language?.args).toEqual(['en']);
  });

  it('passes a language change on as soon as it happens', async () => {
    const { locale } = await load(true);
    resetBindings();

    locale.set('en');
    await new Promise((resolve) => setTimeout(resolve, 0));
    locale.set('ru');
    await new Promise((resolve) => setTimeout(resolve, 0));

    const sent = calls.filter((call) => call.key === 'metadata.SetLanguage').map((call) => call.args[0]);
    expect(sent).toEqual(['en', 'ru']);
  });

  it('sends nothing in a browser preview', async () => {
    const { metadata, locale } = await load(false);

    locale.set('en');
    await metadata.getMetadataView('g1');
    await metadata.findMetadataCandidates('g1');

    expect(calls).toEqual([]);
  });
});

describe('metadata calls', () => {
  it('shows an unresolved placeholder for a game in a browser preview', async () => {
    const { metadata } = await load(false);

    await expect(metadata.getMetadataView('g9')).resolves.toMatchObject({
      game: { id: 'g9' },
      resolved: false,
      match: 'idle',
      screenshots: [],
    });
  });

  it.each([
    ['applyMetadataMatch', ['g1', 'igdb:5'], 'metadata.ApplyMatch'],
    ['dismissMetadataMatch', ['g1'], 'metadata.DismissMatch'],
    ['refreshMetadata', ['g1'], 'metadata.Refresh'],
  ])('%s calls the backend and returns the view it made', async (fn, args, binding) => {
    answer(binding, { game: { id: 'g1' }, resolved: true });
    const { metadata } = await load(true);
    resetBindings();
    answer(binding, { game: { id: 'g1' }, resolved: true });

    await expect(metadata[fn](...args)).resolves.toMatchObject({ resolved: true });

    expect(calls.filter((call) => call.key === binding)).toEqual([{ key: binding, args }]);
  });

  it.each([['applyMetadataMatch', ['g1', 'p']], ['dismissMetadataMatch', ['g1']], ['refreshMetadata', ['g1']]])(
    '%s refuses in a browser preview',
    async (fn, args) => {
      const { metadata } = await load(false);

      await expect(metadata[fn](...args)).rejects.toThrow('unavailable in browser');
    },
  );

  it('asks for no art when there are no games to ask about', async () => {
    const { metadata } = await load(true);
    resetBindings();

    await expect(metadata.getGameArt([])).resolves.toEqual({});
    await expect(metadata.ensureArt([])).resolves.toEqual([]);

    expect(calls).toEqual([]);
  });
});
