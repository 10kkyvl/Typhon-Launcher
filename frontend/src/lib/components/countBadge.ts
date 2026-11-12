/** The cap a count is shown up to; beyond it the badge shows the cap and a plus ("99+"). */
export const COUNT_MAX = 99;

/** What a count badge shows for `count`: nothing for zero, less or not a number. */
export function countShown(count: number, max = COUNT_MAX): string {
  if (!Number.isFinite(count) || count < 1) return '';
  const whole = Math.floor(count);
  return whole > max ? `${max}+` : String(whole);
}
