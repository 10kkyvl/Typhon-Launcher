/** How fast the stage drifts up against the page's scroll. */
const STAGE_DRIFT = 0.5;
/** How far the page scrolls before the stage is gone: before the grid under the header covers it. */
const STAGE_GONE_PX = 220;

/**
 * Where the stage stands for a page scrolled `scrollTop` px down: shifted up by `shift` px (negative)
 * and faded to `opacity`. Above the top (an overscroll) and an unreadable value count as the top.
 */
export function stageDrift(scrollTop: number): { shift: number; opacity: number } {
  const y = Number.isFinite(scrollTop) && scrollTop > 0 ? scrollTop : 0;
  if (y === 0) return { shift: 0, opacity: 1 };
  return { shift: -Math.round(y * STAGE_DRIFT * 10) / 10, opacity: Math.max(0, 1 - y / STAGE_GONE_PX) };
}
