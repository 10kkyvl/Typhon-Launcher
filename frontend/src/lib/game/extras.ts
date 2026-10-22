import type { MessageKey } from '../i18n';
import type { ReferenceKind, SavePaths, SteamDlc, SteamInfo, SteamRequirement } from '../services/metadata';
import type { FactInput } from './view';

export const DLC_COLLAPSED = 8;

export interface RequirementColumns {
  minimum: SteamRequirement[];
  recommended: SteamRequirement[];
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function rows(list: SteamRequirement[] | null | undefined): SteamRequirement[] {
  const out: SteamRequirement[] = [];
  for (const item of list ?? []) {
    const label = text(item?.label);
    const value = text(item?.value);
    if (label && value) out.push({ label, value });
  }
  return out;
}

export function requirementColumns(steam: SteamInfo | null | undefined): RequirementColumns {
  return {
    minimum: rows(steam?.requirements?.minimum),
    recommended: rows(steam?.requirements?.recommended),
  };
}

export function hasRequirements(columns: RequirementColumns): boolean {
  return columns.minimum.length > 0 || columns.recommended.length > 0;
}

export interface DlcView {
  items: SteamDlc[];
  total: number;
  hidden: number;
  collapsible: boolean;
}

export function dlcView(list: SteamDlc[] | null | undefined, expanded: boolean, limit = DLC_COLLAPSED): DlcView {
  const all: SteamDlc[] = [];
  for (const item of list ?? []) {
    const name = text(item?.name);
    if (item && item.appId > 0 && name) all.push({ appId: item.appId, name });
  }
  const collapsible = all.length > limit;
  const items = collapsible && !expanded ? all.slice(0, limit) : all;
  return { items, total: all.length, hidden: all.length - items.length, collapsible };
}

const CONTROLLER: Record<string, MessageKey> = {
  full: 'games.detailControllerFull',
  partial: 'games.detailControllerPartial',
};

export function steamFacts(
  steam: SteamInfo | null | undefined,
  translate: (key: MessageKey) => string,
  count: (value: number) => string,
): FactInput[] {
  if (!steam) return [];
  const controllerKey = CONTROLLER[steam.controller ?? ''];
  const metacritic = steam.metacritic;
  const recommendations = steam.recommendations;
  return [
    { label: translate('games.detailFactController'), value: controllerKey ? translate(controllerKey) : '' },
    {
      label: translate('games.detailFactMetacritic'),
      value: typeof metacritic === 'number' ? String(metacritic) : '',
    },
    {
      label: translate('games.detailFactRecommendations'),
      value: typeof recommendations === 'number' ? count(recommendations) : '',
    },
  ];
}

export function referenceKinds(save: SavePaths | null | undefined, hasTitle: boolean): ReferenceKind[] {
  const out: ReferenceKind[] = [];
  if (text(save?.pageUrl)) out.push('pcgamingwiki');
  if (hasTitle) out.push('howlongtobeat');
  return out;
}

export interface SaveAttribution {
  source: string;
  license: string;
  via: string;
}

export function saveAttribution(
  hint: { source?: string | null; license?: string | null; via?: string | null } | null | undefined,
): SaveAttribution | null {
  const source = text(hint?.source);
  const license = text(hint?.license);
  if (!source || !license) return null;
  return { source, license, via: text(hint?.via) };
}
