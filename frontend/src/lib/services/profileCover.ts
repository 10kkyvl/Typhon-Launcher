import { Service as AccountService } from '../../../bindings/typhon/internal/account';
import { inWails } from './backend';
import { AccountError, toAccountError } from './account';

export async function uploadProfileCover(encoded: string): Promise<string> {
  if (!inWails) throw new AccountError('unauthenticated');
  try {
    return (await AccountService.UploadCover(encoded)).coverUrl;
  } catch (error) { throw toAccountError(error); }
}
