import { describe, expect, it } from 'vitest';
import page from './Settings.svelte?raw';
import flow from './settingsPage.svelte.ts?raw';

describe('Settings send-logs flow', () => {
  it('never sends on the button click itself, only after confirmation', () => {
    expect(page).toMatch(/onclick=\{page\.openSendLogsConfirm\}/);
    expect(page).not.toMatch(/onclick=\{[^}]*sendLogs\(\)[^}]*\}/);
  });

  // The send-logs confirmation goes through the same `pending` queue every
  // other destructive confirmation of the page uses, so there is one
  // ConfirmModal instance rather than one per action.
  it('routes the confirmed send through ConfirmModal', () => {
    expect(flow).toContain('pending = { prompt: sendLogsPrompt(), run: confirmSendLogs }');
    expect(page).toMatch(/\{#if page\.pending\}/);
    expect(page).toContain('<ConfirmModal');
    expect(page).toContain('prompt={page.pending.prompt}');
    expect(page).toContain('onconfirm={page.pending.run}');
  });

  it('only calls sendLogs from inside the confirmed handler', () => {
    expect(flow).toMatch(/function confirmSendLogs\(\)[\s\S]*?void sendLogs\(\)/);
  });

  it('closes the confirmation immediately and shows real progress on the page', () => {
    expect(flow).toMatch(/function confirmSendLogs\(\)[\s\S]*?void sendLogs\(\)/);
    expect(flow).not.toMatch(/await sendLogs\(\)/);
    expect(page).not.toMatch(/await sendLogs\(\)/);
    expect(page).toContain('<ProgressBar');
    expect(page).toContain('value={logsProgressPercent}');
    expect(page).toContain("indeterminate={page.logsStage !== 'sending' || !page.logsProgress}");
  });

  it('shows the short id from a successful send', () => {
    expect(page).toContain('page.logsSendResult.id');
    expect(flow).toContain('logsSendResult = result');
  });

  it('resolves a failed send through the coded error mapper, never raw text', () => {
    expect(flow).toContain('logsUploadErrorText(err)');
    expect(flow).not.toMatch(/logsSendFailure\s*=\s*.*err\.message/);
  });

  it('offers the manual download as a fallback after any failure', () => {
    expect(page).toContain("msg('settings.aboutLogsSendFailedHint')");
  });

  it('warns about dropped rotations when the upload had to trim the bundle', () => {
    expect(page).toContain("msg('settings.aboutLogsSendDroppedNote'");
    expect(page).toContain('page.logsSendResult.dropped');
  });
});
