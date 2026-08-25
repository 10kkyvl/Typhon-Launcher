import { describe, expect, it } from 'vitest';
import { maskEmail } from './email';

describe('maskEmail', () => {
  it.each([
    ['player@example.com', 'p*****@example.com'],
    ['a@example.com', 'a*@example.com'],
    ['nefka2007@gmail.com', 'n********@gmail.com'],
    ['very.long.address.of.a.person@mail.ru', 'v**********@mail.ru'],
    ['user@sub.domain.co.uk', 'u***@sub.domain.co.uk'],
    ['weird@name@example.com', 'w*********@example.com'],
  ])('masks %s', (email, expected) => {
    expect(maskEmail(email)).toBe(expected);
  });

  it.each([
    ['', ''],
    ['   ', ''],
  ])('returns nothing for %o', (email, expected) => {
    expect(maskEmail(email)).toBe(expected);
  });

  it.each([['@example.com'], ['not-an-email'], ['trailing@']])('hides %s entirely', (email) => {
    expect(maskEmail(email)).not.toContain(email.replace('@', ''));
    expect(maskEmail(email)).toMatch(/^\*+$/);
  });

  it('never leaks the local part', () => {
    expect(maskEmail('secretive@gmail.com')).not.toContain('ecretive');
  });
});
