import { msg } from '../i18n';
import type { ChatPeer } from '../services/messaging';
import { getDraft, openChat, setDraft } from '../stores/messaging';

export function inviteText(title: string): string {
  return msg('social.userCommonInviteDraft', { title });
}

export function inviteToPlay(peer: ChatPeer, title: string): void {
  const text = inviteText(title);
  const current = getDraft(peer.id);
  if (!current.includes(text)) setDraft(peer.id, current.trim() ? `${current}\n${text}` : text);
  openChat(peer);
}
