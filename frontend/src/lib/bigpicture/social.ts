export type SocialAuthState = 'bootstrapping' | 'authenticated' | 'unauthenticated' | 'unavailable' | 'guest' | 'offline';

export type SocialAvailability = 'loading' | 'available' | 'guest' | 'offline' | 'sign-in';

export function socialAvailability(state: SocialAuthState): SocialAvailability {
  if (state === 'authenticated') return 'available';
  if (state === 'guest') return 'guest';
  if (state === 'offline') return 'offline';
  if (state === 'bootstrapping') return 'loading';
  return 'sign-in';
}

/** A shared social snapshot is usable only for the account that loaded it. */
export function ownsFriendsSnapshot(currentOwner: string, snapshotOwner: string): boolean {
  return currentOwner !== '' && currentOwner === snapshotOwner;
}

/** Reject late profile and list responses after another selection or account change. */
export function createRequestGate() {
  let revision = 0;
  return {
    begin(): number {
      revision += 1;
      return revision;
    },
    invalidate(): void {
      revision += 1;
    },
    isCurrent(id: number): boolean {
      return id === revision;
    },
  };
}
