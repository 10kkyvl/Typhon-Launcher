import { describe, expect, it } from 'vitest';
import { allowedTokenNamesFixture, cssCases, tokenNameCases, tokenValueCases } from './cases';
import { validateCss, validateTokenName, validateTokenValue } from './validate';

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
