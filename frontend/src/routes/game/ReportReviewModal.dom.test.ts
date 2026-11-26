import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { report } from '../../lib/services/reviews';
import { toasts } from '../../lib/stores/toasts';
import { user } from '../../lib/testing/dom';
import { resetBindings } from '../../lib/testing/fakeBinding';
import { held, sentToGo } from '../../lib/testing/gameDialogsTesting';
import ReportReviewModal from './ReportReviewModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/gameDialogsTesting')).goRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/reviews', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/reviews')>()),
  report: vi.fn(),
}));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  vi.mocked(report).mockReset();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const REASONS = () => [
  msg('reviews.reportReasonSpam'),
  msg('reviews.reportReasonOffensive'),
  msg('reviews.reportReasonSpoilers'),
  msg('reviews.reportReasonOffTopic'),
  msg('reviews.reportReasonOther'),
];

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

function openDialog() {
  const onclose = vi.fn();
  render(ReportReviewModal, { target: app, props: { reviewId: 42, onclose } });
  return { onclose, dialog: screen.getByRole('dialog', { name: msg('reviews.reportTitle') }) };
}

async function chooseAndSend(reason: string) {
  const opened = openDialog();
  const keys = user();
  await keys.click(within(opened.dialog).getByRole('radio', { name: reason }));
  const send = within(opened.dialog).getByRole('button', { name: msg('reviews.reportSubmit') });
  send.focus();
  await keys.keyboard('{Enter}');
  return { ...opened, keys, send };
}

describe('ReportReviewModal: what it offers', () => {
  it('offers the five reasons by name', () => {
    const { dialog } = openDialog();

    for (const reason of REASONS()) expect(within(dialog).getByText(reason)).toBeTruthy();
  });

  it('closes on Escape without sending', async () => {
    const { onclose } = openDialog();

    await user().keyboard('{Escape}');

    expect(onclose).toHaveBeenCalledTimes(1);
    expect(report).not.toHaveBeenCalled();
  });
});

describe('ReportReviewModal: choosing a reason', () => {
  it('is a radio group named by its question, one Tab stop, with the focus on its first reason', () => {
    const { dialog } = openDialog();

    const group = within(dialog).getByRole('radiogroup', { name: msg('reviews.reportReasonLabel') });
    const radios = within(group).getAllByRole('radio');
    expect(radios.map((radio) => radio.textContent?.trim())).toEqual(REASONS());
    expect(radios.filter((radio) => radio.tabIndex === 0)).toHaveLength(1);
    expect(document.activeElement).toBe(radios[0]);
  });

  it('sends nothing while a reason is chosen with the arrows, and holds Send with the reason until one is', async () => {
    const { dialog } = openDialog();
    const keys = user();
    const send = within(dialog).getByRole('button', { name: msg('reviews.reportSubmit') });
    expect(send).toHaveProperty('disabled', false);
    expect(send.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(send)).toContain(msg('reviews.reportChooseReason'));
    await keys.click(send);
    within(dialog).getAllByRole('radio')[0].focus();

    await keys.keyboard('{ArrowDown}{ArrowDown}');

    expect(report).not.toHaveBeenCalled();
    expect(within(dialog).getByRole('radio', { name: msg('reviews.reportReasonSpoilers') }).getAttribute('aria-checked')).toBe('true');
    expect(send.getAttribute('aria-disabled')).toBeNull();
  });
});

describe('ReportReviewModal: sending', () => {
  it('sends the chosen reason once, keeps Send focused and busy, and holds the rest while it runs', async () => {
    const call = held<void>();
    vi.mocked(report).mockReturnValueOnce(call.promise);

    const { dialog, keys, send, onclose } = await chooseAndSend(msg('reviews.reportReasonOffTopic'));
    await keys.keyboard('{Enter}');

    expect(report).toHaveBeenCalledExactlyOnceWith(42, 'off_topic');
    expect(send.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(send);
    expect(within(dialog).getByRole('radiogroup').getAttribute('aria-disabled')).toBe('true');
    const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
    expect(cancel).toHaveProperty('disabled', false);
    expect(describedBy(cancel)).toContain(msg('modals.actionRunning'));
    await keys.click(cancel);
    await keys.keyboard('{Escape}');
    expect(onclose).not.toHaveBeenCalled();

    call.resolve();
    await vi.waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
    expect(get(toasts).map((toast) => toast.message)).toEqual([msg('reviews.reportSent')]);
  });

  it.each([
    ['a code with words', new AccountError('review_report_limit'), () => msg('reviews.errorReportLimit')],
    ['no code of its own', new Error('typhon:reviews.something_new: отказ сервера'), () => msg('reviews.reportFailed')],
  ])('says a refused report (%s) in the dialog, in the launcher\'s words, and moves the focus there', async (_case, err, words) => {
    vi.mocked(report).mockRejectedValueOnce(err);

    const { dialog, onclose } = await chooseAndSend(msg('reviews.reportReasonSpam'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(words());
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(dialog.textContent).not.toContain('отказ сервера');
    expect(get(toasts)).toEqual([]);
    expect(onclose).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true));
  });
});
