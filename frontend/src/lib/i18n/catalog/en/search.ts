import type { Message } from '../../types';
import type { SearchKey } from '../ru/search';

export const search: Record<SearchKey, Message> = {
  'search.unavailable': 'Search unavailable',
  'search.releases': { one: '{count} release', other: '{count} releases' },
  'search.sources': { one: '{count} source', other: '{count} sources' },
  'search.placeholder': 'Search games and releases',
  'search.games': 'Games',
  'search.more': 'and {count} more',
  'search.releasesNoMatch': 'Releases without a match',
  'search.nothingFound': 'Nothing found',
};
