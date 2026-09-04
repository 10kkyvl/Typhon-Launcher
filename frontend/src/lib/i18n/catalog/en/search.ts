import type { Message } from '../../types';
import type { SearchKey } from '../ru/search';

export const search: Record<SearchKey, Message> = {
  'search.releases': { one: '{count} release', other: '{count} releases' },
  'search.sources': { one: '{count} source', other: '{count} sources' },
};
