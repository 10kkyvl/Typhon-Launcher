<script lang="ts">
  import { Paperclip, Send, X } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';
  import { activePeer, attachmentDrafts, sendMessage, setAttachmentDrafts, setDraft, setTyping } from '../stores/messaging';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';
  import { chatErrorText } from '../social/chatErrors';
  import { appendDrafts, checkChatImage, clipboardImages, draftFromFile, draftFromPicked, type ChatAttachmentDraft } from '../social/chatImage';
  import { pickChatImages, uploadChatImage, type Attachment, type ChatPeer } from '../services/messaging';
  import { msg } from '../i18n';

  /**
   * Where a message to `peer` is written: the text, the images attached to it, and Send. The panel
   * holds the draft (it follows the open conversation) and shows a failed send (`onerror`, '' to clear
   * it).
   */
  let {
    peer,
    draft = $bindable(''),
    onerror,
    onsent,
  }: {
    peer: ChatPeer;
    draft?: string;
    onerror: (text: string) => void;
    onsent: () => void;
  } = $props();

  let composer = $state<HTMLTextAreaElement | undefined>(undefined);
  let sending = $state(false);
  let uploadProgress = $state({ current: 0, total: 0 });

  const draftList = $derived($attachmentDrafts[peer.id] ?? []);

  function setComposer(value: string): void {
    draft = value;
    setDraft(peer.id, draft);
    void setTyping(peer.id, draft.trim().length > 0);
  }

  function addDrafts(targetPeerId: string, incoming: ChatAttachmentDraft[]): void {
    const { drafts, overflow } = appendDrafts($attachmentDrafts[targetPeerId] ?? [], incoming);
    setAttachmentDrafts(targetPeerId, drafts);
    if (overflow) toast(msg('chat.too_many_attachments'), 'danger');
  }

  async function attach(): Promise<void> {
    if (sending) return;
    const targetPeerId = peer.id;
    try {
      const images = await pickChatImages();
      const accepted: ChatAttachmentDraft[] = [];
      for (const image of images) {
        const checked = checkChatImage({ type: image.mime, size: image.bytes });
        if (checked.ok) accepted.push(draftFromPicked(image));
        else toast(msg(checked.code), 'danger');
      }
      if (accepted.length > 0) addDrafts(targetPeerId, accepted);
    } catch (err) {
      toast(chatErrorText(err, 'chat.image_unreadable'), 'danger');
    }
  }

  async function onPaste(event: ClipboardEvent): Promise<void> {
    const files = clipboardImages(event.clipboardData?.items);
    if (files.length === 0) return;
    event.preventDefault();
    if (sending) return;
    const targetPeerId = peer.id;
    const accepted: ChatAttachmentDraft[] = [];
    for (const file of files) {
      const checked = checkChatImage(file);
      if (!checked.ok) {
        toast(msg(checked.code), 'danger');
        continue;
      }
      try {
        accepted.push(await draftFromFile(file));
      } catch (err) {
        toast(chatErrorText(err, 'chat.image_unreadable'), 'danger');
      }
    }
    if (accepted.length > 0) addDrafts(targetPeerId, accepted);
  }

  function removeAttachment(key: string): void {
    if (!sending) setAttachmentDrafts(peer.id, draftList.filter((item) => item.key !== key));
  }

  async function uploadDrafts(targetPeerId: string, pending: ChatAttachmentDraft[]): Promise<Attachment[] | null> {
    let list = pending;
    const total = list.filter((item) => !item.uploaded).length;
    let done = 0;
    uploadProgress = { current: 0, total };
    for (const item of pending) {
      if (item.uploaded) continue;
      uploadProgress = { current: done + 1, total };
      try {
        const uploaded = await uploadChatImage(item.encoded);
        list = list.map((entry) => (entry.key === item.key ? { ...entry, uploaded } : entry));
        setAttachmentDrafts(targetPeerId, list);
        done += 1;
      } catch (err) {
        toast(chatErrorText(err, 'social.chatSendError'), 'danger');
        return null;
      }
    }
    return list.map((item) => ({ ...(item.uploaded as Attachment), url: item.previewUrl }));
  }

  async function submit(): Promise<void> {
    const pending = draftList;
    if ((!draft.trim() && pending.length === 0) || sending) return;
    const targetPeerId = peer.id;
    const submitted = draft.trim();
    sending = true;
    onerror('');
    try {
      let attachments: Attachment[] = [];
      if (pending.length > 0) {
        const uploaded = await uploadDrafts(targetPeerId, pending);
        if (!uploaded) return;
        attachments = uploaded;
      }
      await sendMessage(targetPeerId, submitted, undefined, attachments);
      if (attachments.length > 0) setAttachmentDrafts(targetPeerId, []);
      if ($activePeer?.id === targetPeerId && draft.trim() === submitted) draft = '';
      if ($activePeer?.id === targetPeerId) {
        composer?.focus();
        onsent();
      }
    } catch (err) {
      onerror(chatErrorText(err, 'social.chatSendError'));
    } finally {
      uploadProgress = { current: 0, total: 0 };
      sending = false;
    }
  }

  /** The text field, for the panel to put focus in when a conversation opens. */
  export function field(): HTMLElement | undefined {
    return composer;
  }

  function onKeydown(event: KeyboardEvent): void {
    // The Enter that ends an input method's composition picks the text; it does not send.
    if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
    event.preventDefault();
    void submit();
  }
</script>

{#if draftList.length > 0}
  <div class="draft-list">
    {#each draftList as item (item.key)}
      <div class="draft-chip">
        <img src={item.previewUrl} alt={msg('social.chatImageLabel')} />
        <span class="draft-copy">
          <strong>{item.name}</strong>
          <span>{bytesSize(item.bytes)}</span>
        </span>
        <IconButton size="sm" label={msg('social.chatAttachRemove')} disabled={sending} onclick={() => removeAttachment(item.key)}><X size="1.4rem" /></IconButton>
      </div>
    {/each}
  </div>
  {#if uploadProgress.total > 0}
    <div class="upload-status" role="status">{uploadProgress.total === 1 ? msg('social.chatUploading') : msg('social.chatUploadProgress', { current: uploadProgress.current, total: uploadProgress.total })}</div>
  {/if}
{/if}
<div class="composer">
  <IconButton label={msg('social.chatAttach')} hint={msg('social.chatAttachHint')} disabled={sending} onclick={() => attach()}><Paperclip size="1.7rem" /></IconButton>
  <textarea
    class="input"
    bind:this={composer}
    value={draft}
    maxlength="2000"
    rows="2"
    placeholder={msg('social.chatPlaceholder')}
    disabled={sending}
    oninput={(event) => setComposer((event.currentTarget as HTMLTextAreaElement).value)}
    onkeydown={onKeydown}
    onpaste={onPaste}
  ></textarea>
  <IconButton variant="primary" label={msg('social.chatSend')} disabled={(!draft.trim() && draftList.length === 0) || sending} onclick={() => submit()}><Send size="1.7rem" /></IconButton>
</div>
<div class="composer-hint">{msg('social.chatHint')}</div>

<style>
  .composer { display: flex; align-items: flex-end; gap: 0.7rem; padding: 0.8rem 1rem 0; border-top: 1px solid var(--border); }
  .composer textarea { flex: 1; min-height: 4.2rem; height: auto; resize: none; padding: 0.7rem; font: inherit; font-size: var(--font-sm); }
  .draft-list { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; margin: 0.8rem 1rem 0; }
  .draft-chip { display: flex; align-items: center; gap: 0.6rem; min-width: 0; padding: 0.4rem 0.5rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface-3); }
  .draft-chip img { width: 3.2rem; height: 3.2rem; flex-shrink: 0; border-radius: var(--radius-sm); object-fit: cover; }
  .draft-copy { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.15rem; }
  .draft-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--font-xs); }
  .draft-copy span { color: var(--text-3); font-size: 1rem; }
  .upload-status { margin: 0.5rem 1rem 0; color: var(--text-3); font-size: 1rem; }
  .composer-hint { padding: 0.45rem 1rem 0.8rem; color: var(--text-3); font-size: 1rem; }
</style>
