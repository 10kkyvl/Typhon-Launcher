import { describe, expect, it } from 'vitest';
import type { MessageKey } from '../i18n';
import { msg } from '../i18n';
import { metadataErrorText } from '../metadata/metadataErrors';
import type { SteamInfo } from '../services/metadata';
import { DLC_COLLAPSED, dlcView, hasRequirements, referenceKinds, requirementColumns, saveAttribution, steamFacts } from './extras';
import { facts } from './view';

const translate = (key: MessageKey) => key;
const count = (value: number) => `#${value}`;

function steam(patch: Partial<SteamInfo> = {}): SteamInfo {
  return { appId: 10, url: 'https://store.steampowered.com/app/10', ...patch };
}

function dlc(n: number) {
  return Array.from({ length: n }, (_, i) => ({ appId: i + 1, name: `DLC ${i + 1}` }));
}

describe('requirementColumns', () => {
  it('survives null lists and a missing block', () => {
    expect(requirementColumns(undefined)).toEqual({ minimum: [], recommended: [] });
    expect(requirementColumns(steam())).toEqual({ minimum: [], recommended: [] });
    expect(requirementColumns(steam({ requirements: { minimum: null, recommended: null } }))).toEqual({
      minimum: [],
      recommended: [],
    });
  });

  it('keeps rows verbatim as plain text and drops empty ones', () => {
    const columns = requirementColumns(
      steam({
        requirements: {
          minimum: [
            { label: 'ОС', value: ' Windows 10 ' },
            { label: 'Память', value: '' },
            { label: '', value: '8 GB' },
            { label: 'Заметки', value: '<b>x</b>' },
          ],
          recommended: null,
        },
      }),
    );
    expect(columns.minimum).toEqual([
      { key: 'ОС#0', label: 'ОС', value: 'Windows 10' },
      { key: 'Заметки#0', label: 'Заметки', value: '<b>x</b>' },
    ]);
    expect(columns.recommended).toEqual([]);
  });

  it('keeps rows that repeat a label under distinct keys', () => {
    const columns = requirementColumns(
      steam({
        requirements: {
          minimum: [
            { label: 'Видеокарта', value: 'DirectX 8' },
            { label: 'ОС', value: 'Windows XP' },
            { label: 'Видеокарта', value: '16 MB' },
          ],
          recommended: null,
        },
      }),
    );
    expect(columns.minimum.map((row) => row.value)).toEqual(['DirectX 8', 'Windows XP', '16 MB']);
    expect(new Set(columns.minimum.map((row) => row.key)).size).toBe(3);
  });

  it('reports a block as present when only one column has rows', () => {
    expect(hasRequirements({ minimum: [], recommended: [] })).toBe(false);
    expect(hasRequirements({ minimum: [], recommended: [{ key: 'a#0', label: 'a', value: 'b' }] })).toBe(true);
  });
});

describe('dlcView', () => {
  it('handles null and entries without a usable id or name', () => {
    expect(dlcView(null, false).items).toEqual([]);
    const view = dlcView(
      [
        { appId: 0, name: 'zero' },
        { appId: 5, name: '  ' },
        { appId: 6, name: 'ok' },
      ],
      false,
    );
    expect(view.items).toEqual([{ appId: 6, name: 'ok' }]);
    expect(view.total).toBe(1);
  });

  it('keeps a list of exactly the limit expanded', () => {
    const view = dlcView(dlc(DLC_COLLAPSED), false);
    expect(view).toMatchObject({ total: 8, hidden: 0, collapsible: false });
    expect(view.items).toHaveLength(8);
  });

  it('collapses a longer list to the limit and expands it on request', () => {
    const collapsed = dlcView(dlc(11), false);
    expect(collapsed).toMatchObject({ total: 11, hidden: 3, collapsible: true });
    expect(collapsed.items).toHaveLength(8);
    const expanded = dlcView(dlc(11), true);
    expect(expanded).toMatchObject({ total: 11, hidden: 0, collapsible: true });
    expect(expanded.items).toHaveLength(11);
  });
});

describe('steamFacts', () => {
  it('returns nothing without steam data', () => {
    expect(steamFacts(null, translate, count)).toEqual([]);
    expect(facts(steamFacts(steam(), translate, count))).toEqual([]);
  });

  it('labels full and partial gamepad support and ignores unknown values', () => {
    expect(facts(steamFacts(steam({ controller: 'full' }), translate, count))).toEqual([
      { label: 'games.detailFactController', value: 'games.detailControllerFull' },
    ]);
    expect(facts(steamFacts(steam({ controller: 'partial' }), translate, count))[0].value).toBe(
      'games.detailControllerPartial',
    );
    expect(facts(steamFacts(steam({ controller: 'none' }), translate, count))).toEqual([]);
    expect(facts(steamFacts(steam({ controller: '' }), translate, count))).toEqual([]);
  });

  it('shows numbers through the formatter, zero included, and skips null', () => {
    const shown = facts(steamFacts(steam({ metacritic: 0, recommendations: 12345 }), translate, count));
    expect(shown).toEqual([
      { label: 'games.detailFactMetacritic', value: '0' },
      { label: 'games.detailFactRecommendations', value: '#12345' },
    ]);
    expect(facts(steamFacts(steam({ metacritic: null, recommendations: null }), translate, count))).toEqual([]);
  });
});

describe('referenceKinds', () => {
  it('offers PCGamingWiki only when the save data returned a page', () => {
    const save = { pageUrl: 'https://www.pcgamingwiki.com/wiki/X', windows: null, source: '', license: '' };
    expect(referenceKinds(save, true)).toEqual(['pcgamingwiki', 'howlongtobeat']);
    expect(referenceKinds(null, true)).toEqual(['howlongtobeat']);
    expect(referenceKinds({ ...save, pageUrl: ' ' }, true)).toEqual(['howlongtobeat']);
  });

  it('offers nothing for a game without a title', () => {
    expect(referenceKinds(null, false)).toEqual([]);
  });
});

describe('reference and save data errors', () => {
  const cases: [string, MessageKey][] = [
    ['metadata.no_save_data', 'errMetadata.metadataNoSaveData'],
    ['metadata.save_data_invalid', 'errMetadata.metadataSaveDataInvalid'],
    ['metadata.game_not_resolved', 'errMetadata.metadataGameNotResolved'],
    ['metadata.link_kind_unknown', 'errMetadata.metadataLinkKindUnknown'],
    ['metadata.link_no_data', 'errMetadata.metadataLinkNoData'],
    ['metadata.link_rejected', 'errMetadata.metadataLinkRejected'],
  ];

  it.each(cases)('maps %s to its own text', (code, key) => {
    expect(metadataErrorText(new Error(`typhon:${code}`), 'fallback')).toBe(msg(key));
  });

  it('keeps the fallback for an unrelated error', () => {
    expect(metadataErrorText(new Error('network down'), 'fallback')).toBe('fallback');
  });
});

describe('saveAttribution', () => {
  it('gives no caption without a hint', () => {
    expect(saveAttribution(null)).toBeNull();
    expect(saveAttribution(undefined)).toBeNull();
  });

  it('gives no caption when the source or license is missing', () => {
    expect(saveAttribution({ source: 'PCGamingWiki', license: ' ' })).toBeNull();
    expect(saveAttribution({ source: '', license: 'CC BY-NC-SA 3.0' })).toBeNull();
  });

  it('takes the texts from the hint', () => {
    expect(saveAttribution({ source: 'PCGamingWiki', license: 'CC BY-NC-SA 3.0', via: 'Ludusavi Manifest' })).toEqual({
      source: 'PCGamingWiki',
      license: 'CC BY-NC-SA 3.0',
      via: 'Ludusavi Manifest',
    });
    expect(saveAttribution({ source: 'PCGamingWiki', license: 'CC BY-NC-SA 3.0' })?.via).toBe('');
  });
});
