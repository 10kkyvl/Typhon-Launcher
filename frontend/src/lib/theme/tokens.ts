import type { MessageKey } from '../i18n';

export type TokenGroupId = 'background' | 'surfaces' | 'text' | 'accent' | 'status' | 'sizes' | 'effects';

export interface TokenGroup {
  id: TokenGroupId;
  colors: boolean;
  collapsed: boolean;
  tokens: readonly string[];
}

export const tokenGroups: readonly TokenGroup[] = [
  { id: 'background', colors: true, collapsed: false, tokens: ['--bg', '--bg-sidebar'] },
  {
    id: 'surfaces',
    colors: true,
    collapsed: false,
    tokens: [
      '--surface',
      '--surface-2',
      '--surface-3',
      '--surface-4',
      '--border',
      '--border-strong',
      '--hover',
      '--hover-strong',
    ],
  },
  { id: 'text', colors: true, collapsed: false, tokens: ['--text', '--text-2', '--text-3'] },
  {
    id: 'accent',
    colors: true,
    collapsed: false,
    tokens: ['--accent', '--accent-hover', '--accent-subtle', '--accent-text', '--accent-ring'],
  },
  {
    id: 'status',
    colors: true,
    collapsed: false,
    tokens: ['--success', '--success-subtle', '--warning', '--warning-subtle', '--danger', '--danger-subtle'],
  },
  {
    id: 'sizes',
    colors: false,
    collapsed: true,
    tokens: [
      '--radius-xs',
      '--radius-sm',
      '--radius-md',
      '--radius-lg',
      '--radius-xl',
      '--cut',
      '--space-1',
      '--space-2',
      '--space-3',
      '--space-4',
      '--space-5',
      '--space-6',
      '--space-8',
      '--space-10',
      '--space-12',
      '--page-x',
      '--page-max',
      '--prose-max',
      '--sidebar-w',
      '--topbar-h',
      '--font-xs',
      '--font-sm',
      '--font-md',
      '--font-lg',
      '--font-xl',
      '--font-title',
      '--font-hero',
      '--tracking-title',
      '--tracking-heading',
      '--control-sm',
      '--control-md',
      '--control-lg',
    ],
  },
  {
    id: 'effects',
    colors: false,
    collapsed: true,
    tokens: [
      '--font-sans',
      '--ease',
      '--ease-spring',
      '--dur',
      '--dur-fast',
      '--dur-panel',
      '--dur-slow',
      '--shadow-pop',
      '--shadow-modal',
      '--shadow-lift',
    ],
  },
];

export const groupedTokenNames: ReadonlySet<string> = new Set(tokenGroups.flatMap((g) => g.tokens));

export function tokenKey(name: string): string {
  return name.replace(/^--/, '');
}

export function tokenLabelKey(name: string): MessageKey {
  return `settings.themeToken.${tokenKey(name)}` as MessageKey;
}

export function tokenHintKey(name: string): MessageKey {
  return `settings.themeTokenHint.${tokenKey(name)}` as MessageKey;
}

export function groupTitleKey(id: TokenGroupId | 'other'): MessageKey {
  return `settings.themeGroup.${id}` as MessageKey;
}

export function isHexColor(value: string): boolean {
  return /^#[0-9a-fA-F]{6}$/.test(value.trim());
}
