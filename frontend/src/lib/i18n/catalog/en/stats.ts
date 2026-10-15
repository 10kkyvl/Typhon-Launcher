import type { Message } from '../../types';
import type { StatsKey } from '../ru/stats';

export const stats: Record<StatsKey, Message> = {
  'stats.nav': 'Statistics',
  'stats.title': 'Statistics',
  'stats.subtitle': 'How much you played over the chosen period',
  'stats.periodLabel': 'Period',
  'stats.periodWeek': 'Week',
  'stats.periodMonth': 'Month',
  'stats.periodYear': 'Year',
  'stats.totalLabel': 'Total played',
  'stats.sessionsLabel': 'Sessions',
  'stats.longestLabel': 'Longest session',
  'stats.longestSub': '{game} · {date}',
  'stats.longestNone': 'No data',
  'stats.chartTitle': 'By day',
  'stats.chartTitleYear': 'By month',
  'stats.chartIdle': 'no play',
  'stats.topTitle': 'Most played',
  'stats.topSessions': { one: '{count} session', other: '{count} sessions' },
  'stats.deletedGame': 'Deleted game',
  'stats.empty': 'No games were played in this period. Launch a game and your statistics will appear here.',
  'stats.loadError': 'Could not load statistics',
  'stats.loadErrorReason': 'Could not load statistics: {reason}',
  'stats.retry': 'Retry',
  'stats.loading':'Calculating statistics',
};
