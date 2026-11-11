import { describe, expect, it } from 'vitest';
import { allowedTokenNamesFixture, cssCases, tokenNameCases, tokenValueCases } from './cases';
import { validateCss, validateTokenName, validateTokenValue, validateTokens } from './validate';
import { tokenRangeText } from './tokenRanges';

describe('validateTokenName', () => {
  it.each(tokenNameCases)('$title', ({ name, valid }) => {
    const error = validateTokenName(name, allowedTokenNamesFixture);
    if (valid) expect(error).toBeNull();
    else expect(error).not.toBeNull();
  });

  it('accepts a Set as well as an array', () => {
    expect(validateTokenName('--bg', new Set(allowedTokenNamesFixture))).toBeNull();
  });
});

describe('validateTokenValue', () => {
  it.each(tokenValueCases)('$title', ({ value, valid }) => {
    const error = validateTokenValue(value);
    if (valid) expect(error).toBeNull();
    else expect(error).not.toBeNull();
  });
});

describe('validateCss', () => {
  it.each(cssCases)('$title', ({ css, valid }) => {
    const error = validateCss(css);
    if (valid) expect(error).toBeNull();
    else expect(error).not.toBeNull();
  });
});

describe('validateCss mirrors internal/theme', () => {
  const goTheme = Object.values(
    import.meta.glob<string>('../../../../internal/theme/theme.go', { query: '?raw', import: 'default', eager: true }),
  )[0];
  const block = /forbiddenCSSSubstrings = \[\]string\{([\s\S]*?)\n\}/.exec(goTheme)?.[1] ?? '';
  const goForbidden = [...block.matchAll(/"([^"]*)"/g)].map((m) => m[1]);

  it('reads the Go list', () => {
    expect(goForbidden.length).toBeGreaterThan(10);
  });

  it.each(goForbidden)('refuses %s like the backend does', (substring) => {
    expect(validateCss(`.a { b: ${substring} }`)).not.toBeNull();
  });

  it.each([
    ['escape', String.raw`.a { background: u\72l(x) }`],
    ['space before paren', '.a { background: image-set ("https://x.test/a.png" 1x) }'],
    ['comment inside a name', '.a { background: image/**/-set("https://x.test/a.png" 1x) }'],
    ['upper case', '.a { background: IMAGE-SET("https://x.test/a.png" 1x) }'],
  ])('refuses the %s bypass', (_name, css) => {
    expect(validateCss(css)).not.toBeNull();
  });

  it('keeps ordinary rules and data-part selectors', () => {
    expect(validateCss('[data-part="sidebar-item"][data-id="stats"] { order: -1; color: var(--accent); }')).toBeNull();
  });
});

describe('validateTokens', () => {
  it('reports a ranged token outside its range in the draft, before saving', () => {
    const problems = validateTokens({ '--ambient-dim': '2', '--bg': '#000' }, ['--ambient-dim', '--bg']);
    expect(problems).toEqual([tokenRangeText({ name: '--ambient-dim', min: 0, max: 1 })]);
  });

  it('accepts a ranged token inside its range, and keeps the name and value checks', () => {
    expect(validateTokens({ '--ambient-dim': '0.6' }, ['--ambient-dim'])).toEqual([]);
    expect(validateTokens({ '--nope': '1' }, ['--ambient-dim'])).toEqual([validateTokenName('--nope', ['--ambient-dim'])]);
    expect(validateTokens({ '--bg': '' }, ['--bg'])).toHaveLength(1);
  });
});
