/** The ids of the page's section headings: each names its section, and the meter and focus find it by them. */
export const usageSectionIds = {
  games: 'diskusage-games',
  downloads: 'diskusage-downloads',
  leftovers: 'diskusage-leftovers',
  screenshots: 'diskusage-screenshots',
  snapshots: 'diskusage-snapshots',
} as const;

export type UsageSection = keyof typeof usageSectionIds;
