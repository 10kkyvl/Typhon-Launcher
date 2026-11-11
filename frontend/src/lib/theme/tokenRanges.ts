import { msg, type MessageKey } from '../i18n';

export interface TokenRange {
  min: number;
  max: number;
  /** The editor's spin step. */
  step: number;
}

/** The number tokens that are bounded; internal/theme/tokens.go `numberRanges` holds the same (parity test). */
export const TOKEN_RANGES: Readonly<Record<string, TokenRange>> = {
  '--ambient-dim': { min: 0, max: 1, step: 0.01 },
};

export interface RangeProblem {
  name: string;
  min: number;
  max: number;
}

// The theme validator's number syntax (internal/theme/values.go numberRe).
const NUMBER = /^-?(?:\d+(?:\.\d+)?|\.\d+)$/;

/** What is wrong with a ranged token's value (not a plain number, or outside the range); null when nothing is. */
export function tokenRangeProblem(name: string, value: string): RangeProblem | null {
  const range = TOKEN_RANGES[name];
  if (!range) return null;
  const text = value.trim();
  const n = Number(text);
  if (NUMBER.test(text) && n >= range.min && n <= range.max) return null;
  return { name, min: range.min, max: range.max };
}

// The key arrives with the catalog strings; until then the lookup falls back like any missing key.
export const TOKEN_RANGE_KEY: MessageKey = 'theme.token_range';

export function tokenRangeText(problem: RangeProblem): string {
  return msg(TOKEN_RANGE_KEY, { name: problem.name, min: problem.min, max: problem.max });
}
