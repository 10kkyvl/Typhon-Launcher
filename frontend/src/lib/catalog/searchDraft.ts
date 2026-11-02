export interface SearchField {
  field: string;
  search: string;
  draft: boolean;
}

export type SearchRun = 'none' | 'debounced' | 'now';
export interface SearchStep extends SearchField { run: SearchRun }

/**
 * Text from a typhon:// link is a draft: it fills the field and stays on this
 * machine until the user asks to search. `search` is what requests carry.
 */
export function openSearchField(params: Record<string, string>): SearchField {
  const search = params.q ?? '';
  const draft = params.draft ?? '';
  return draft ? { field: draft, search, draft: true } : { field: search, search, draft: false };
}

export function typeSearch(state: SearchField): SearchStep {
  if (!state.draft) return { ...state, search: state.field, run: 'debounced' };
  return { ...state, draft: state.field.trim() !== '', run: 'none' };
}

export function confirmSearch(state: SearchField): SearchStep {
  if (!state.draft) return { ...state, run: 'none' };
  return { ...state, search: state.field.trim(), draft: false, run: 'now' };
}
