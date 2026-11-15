/**
 * The ids of the downloads screen's section headings, in the page's order: what a section's
 * `DownloadsSection` takes as `id`, and where focus goes when the last focused row of a section leaves
 * (`utils/focusHandOff.ts`), before the page heading.
 */
export const downloadsSectionIds = {
  active: 'downloads-active-title',
  installing: 'downloads-installing-title',
  failed: 'downloads-failed-title',
  queue: 'downloads-queue-title',
  completed: 'downloads-completed-title',
} as const;
