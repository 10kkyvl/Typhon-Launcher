/** One choice of a segmented control. */
export interface SegmentOption {
  id: string;
  label: string;
  disabled?: boolean;
  /** Shown after the label and read with it: how many items the choice would show. */
  count?: number;
}
