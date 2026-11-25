/** One row of an OptionList. */
export interface OptionItem {
  id: string;
  /** The row's name; the first line of a row drawn by the list. */
  title: string;
  /** A second line under the title (the folder of an executable, a warning). */
  note?: string;
  /** A short fact at the row's end (a size, a year, a score). */
  fact?: string;
  /** Why the row cannot be chosen now: shown on the row and read as its description. */
  disabledReason?: string;
}
