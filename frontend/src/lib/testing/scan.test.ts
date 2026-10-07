import { describe, expect, it } from 'vitest';
import { balanced, keyCalls, secondArgument } from './scan';

describe('balanced', () => {
  it('returns what sits between the matching parentheses', () => {
    const text = 'call(a, (b), c) tail';

    expect(balanced(text, text.indexOf('('))).toBe('a, (b), c');
  });

  it('is not fooled by a parenthesis inside a string', () => {
    const text = `call('a)', "b(", \`c)\`) tail`;

    expect(balanced(text, text.indexOf('('))).toBe(`'a)', "b(", \`c)\``);
  });

  it('reads through a template literal with an expression in it', () => {
    const text = 'call(`x ${f(1)} y`, 2) tail';

    expect(balanced(text, text.indexOf('('))).toBe('`x ${f(1)} y`, 2');
  });

  it('skips an escaped quote', () => {
    const text = String.raw`call('it\'s )', 1) tail`;

    expect(balanced(text, text.indexOf('('))).toBe(String.raw`'it\'s )', 1`);
  });

  it('gives up on text that never closes', () => {
    expect(balanced('call(a, (b)', 4)).toBeNull();
    expect(balanced("call('open", 4)).toBeNull();
  });
});

describe('keyCalls', () => {
  const files = {
    'a.svelte': `{$t('common.cancel')} {msg("games.title", { count: 2 })} {bp(\`bp.home.title\`)}`,
    'b.ts': `foo.msg('not.a.call'); notmsg('nor.this'); t(variable); const t = 1; t('x.y')`,
    'c.ts': `msg('has.params', { a: 1, b: fn(2) }) + msg('has.none')`,
  };

  it('finds every translation call with a literal key', () => {
    const keys = keyCalls(files, ['$t', 't', 'msg', 'bp']).map(({ file, key }) => `${file}:${key}`);

    expect(keys).toEqual([
      'a.svelte:common.cancel',
      'a.svelte:games.title',
      'a.svelte:bp.home.title',
      'b.ts:x.y',
      'c.ts:has.params',
      'c.ts:has.none',
    ]);
  });

  it('leaves out a method call and a longer function name that merely ends like one', () => {
    const keys = keyCalls({ 'x.ts': `foo.msg('a.b'); notmsg('c.d'); xt('e.f')` }, ['$t', 't', 'msg', 'bp']);

    expect(keys).toEqual([]);
  });

  it('keeps the arguments so the parameters can be inspected', () => {
    const calls = keyCalls(files, ['msg']);
    const withParams = calls.find((call) => call.key === 'has.params')!;
    const without = calls.find((call) => call.key === 'has.none')!;

    expect(secondArgument(withParams.args, withParams.key)).toBe('{ a: 1, b: fn(2) }');
    expect(secondArgument(without.args, without.key)).toBeNull();
  });
});
