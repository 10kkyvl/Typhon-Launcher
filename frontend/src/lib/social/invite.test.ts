import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../stores/messaging', () => {
  const drafts = new Map<string, string>();
  return {
    getDraft: vi.fn((id: string) => drafts.get(id) ?? ''),
    setDraft: vi.fn((id: string, value: string) => void drafts.set(id, value)),
    openChat: vi.fn(),
    sendMessage: vi.fn(),
    drafts,
  };
});

const messaging = await import('../stores/messaging');
const { inviteToPlay } = await import('./invite');

const peer = { id: 'u1', username: 'alice', displayName: 'Alice', avatarUrl: '' };

describe('inviteToPlay', () => {
  beforeEach(() => {
    (messaging as unknown as { drafts: Map<string, string> }).drafts.clear();
    vi.mocked(messaging.openChat).mockClear();
    vi.mocked(messaging.setDraft).mockClear();
  });

  it('кладёт черновик до открытия чата и ничего не отправляет', () => {
    inviteToPlay(peer, 'Portal 2');

    expect(messaging.setDraft).toHaveBeenCalledWith('u1', 'Сыграем в Portal 2?');
    expect(messaging.openChat).toHaveBeenCalledWith(peer);
    expect(vi.mocked(messaging.setDraft).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(messaging.openChat).mock.invocationCallOrder[0],
    );
    expect(messaging.sendMessage).not.toHaveBeenCalled();
  });

  it('не затирает уже набранный текст', () => {
    vi.mocked(messaging.setDraft)('u1', 'привет');
    vi.mocked(messaging.setDraft).mockClear();

    inviteToPlay(peer, 'Portal 2');

    expect(messaging.setDraft).toHaveBeenCalledWith('u1', 'привет\nСыграем в Portal 2?');
  });

  it('повторное нажатие не дублирует приглашение', () => {
    inviteToPlay(peer, 'Portal 2');
    vi.mocked(messaging.setDraft).mockClear();

    inviteToPlay(peer, 'Portal 2');

    expect(messaging.setDraft).not.toHaveBeenCalled();
  });
});
