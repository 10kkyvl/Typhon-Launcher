import type { BigPictureCommand } from './input';

export interface TextRequest {
  title: string;
  initialValue?: string;
  maxLength?: number;
}

export type RequestText = (request: TextRequest) => Promise<string | null>;

export interface BigPicturePageHandle {
  /** Consume local navigation (such as closing a dialog) before shell navigation. */
  handleCommand?: (command: BigPictureCommand) => boolean;
}
