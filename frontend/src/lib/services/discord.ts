import { Service as AccountService } from '../../../bindings/typhon/internal/account';
import { AccountError, toAccountError } from './account';
import { inWails } from './backend';

export interface DiscordLinkCode {
  code: string;
  expiresAt: string;
}

export interface DiscordLink {
  linked: boolean;
  discordId: string;
  discordUsername: string;
  linkedAt: string;
}

export interface DiscordApi {
  createLinkCode(): Promise<DiscordLinkCode>;
  getLink(): Promise<DiscordLink>;
  unlink(): Promise<void>;
}

async function call<T>(run: () => Promise<T>): Promise<T> {
  if (!inWails) throw new AccountError('unauthenticated');
  try {
    return await run();
  } catch (err) {
    throw toAccountError(err);
  }
}

export const discordApi: DiscordApi = {
  createLinkCode: () => call(async () => (await AccountService.CreateDiscordLinkCode()) as unknown as DiscordLinkCode),
  getLink: () => call(async () => (await AccountService.GetDiscordLink()) as unknown as DiscordLink),
  unlink: () => call(() => AccountService.UnlinkDiscord()),
};

export async function openDiscordInvite(): Promise<void> {
  await call(() => AccountService.OpenDiscordInvite());
}
