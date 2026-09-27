import { msg } from '../i18n';

const RESERVED_TOKEN_NAMES = new Set(['--ui-scale']);

export function validateTokenName(name: string, allowed: Set<string> | string[]): string | null {
  if (name.length === 0) return msg('settings.appearanceTokenNameEmpty');
  if (RESERVED_TOKEN_NAMES.has(name)) return msg('settings.appearanceTokenReserved', { name });
  const set = allowed instanceof Set ? allowed : new Set(allowed);
  if (!set.has(name)) return msg('settings.appearanceTokenUnknown', { name });
  return null;
}

const FORBIDDEN_VALUE_SUBSTRINGS = [';', '{', '}', '/*', '\\', 'url(', '@', 'expression('];

export function validateTokenValue(value: string): string | null {
  if (value.length === 0) return msg('settings.appearanceTokenValueEmpty');
  if (value.length > 120) return msg('settings.appearanceTokenValueTooLong');
  const lower = value.toLowerCase();
  for (const substring of FORBIDDEN_VALUE_SUBSTRINGS) {
    if (lower.includes(substring)) return msg('settings.appearanceTokenValueForbidden', { substring });
  }
  return null;
}

const CSS_MAX_BYTES = 32 * 1024;

const FORBIDDEN_CSS_SUBSTRINGS = [
  '@import',
  'url(',
  'expression(',
  'javascript:',
  'behavior:',
  '-moz-binding',
  '</style',
  '--ui-scale',
];

export function validateCss(css: string): string | null {
  if (new TextEncoder().encode(css).length > CSS_MAX_BYTES) return msg('settings.appearanceCssTooLong');
  const lower = css.toLowerCase();
  for (const substring of FORBIDDEN_CSS_SUBSTRINGS) {
    if (lower.includes(substring)) return msg('settings.appearanceCssForbidden', { substring });
  }
  let balance = 0;
  for (const char of css) {
    if (char === '{') balance += 1;
    else if (char === '}') {
      balance -= 1;
      if (balance < 0) return msg('settings.appearanceCssExtraClosingBrace');
    }
  }
  if (balance !== 0) return msg('settings.appearanceCssUnclosedBrace');
  return null;
}
