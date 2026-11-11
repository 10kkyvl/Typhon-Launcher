/** An option of a Select: `id` is the value choosing it sets, `label` what the list and the field show. */
export interface SelectOption {
  id: string;
  label: string;
  /** Shown and announced, skipped by the keys and typeahead, not chosen when pressed. */
  disabled?: boolean;
}
