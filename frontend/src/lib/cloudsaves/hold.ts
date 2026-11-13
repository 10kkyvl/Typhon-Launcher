import { errorCode } from '../i18n/errors';

// The codes of a launch the cloud saves held back for the player. Go keeps the
// same list (savesHoldCodes); TestSavesHoldCodesMatchTheFrontend compares them.
export const SAVES_HOLD_CODES: readonly string[] = ['cloud.conflict_unresolved', 'cloud.syncing'];

export function isSavesHoldCode(code: string): boolean {
  return SAVES_HOLD_CODES.includes(code);
}

export function savesHeldBack(err: unknown): boolean {
  return isSavesHoldCode(errorCode(err));
}
