import { describe, expect, it } from 'vitest';
import source from './Settings.svelte?raw';

describe('Settings send-logs flow', () => {
  it('never sends on the button click itself, only after confirmation', () => {
    expect(source).toMatch(/onclick=\{openSendLogsConfirm\}/);
    expect(source).not.toMatch(/onclick=\{[^}]*sendLogs\(\)[^}]*\}/);
  });

  // The send-logs confirmation goes through the same `pending` queue every
  // other destructive confirmation in this file uses, so there is one
  // ConfirmModal instance rather than one per action.
  it('routes the confirmed send through ConfirmModal', () => {
    expect(source).toContain('pending = { prompt: sendLogsPrompt(), run: confirmSendLogs }');
    expect(source).toMatch(/\{#if pending\}/);
    expect(source).toContain('<ConfirmModal');
    expect(source).toContain('prompt={pending.prompt}');
    expect(source).toContain('onconfirm={pending.run}');
  });

  it('only calls sendLogs from inside the confirmed handler', () => {
    expect(source).toMatch(/function confirmSendLogs\(\)[\s\S]*?void sendLogs\(\)/);
  });

  it('closes the confirmation immediately and shows real progress on the page', () => {
    expect(source).toMatch(/function confirmSendLogs\(\)[\s\S]*?void sendLogs\(\)/);
    expect(source).not.toMatch(/await sendLogs\(\)/);
    expect(source).toContain('<ProgressBar');
    expect(source).toContain('value={logsProgressPercent}');
    expect(source).toContain("indeterminate={logsStage !== 'sending' || !logsProgress}");
  });

  it('shows the short id from a successful send', () => {
    expect(source).toContain('logsSendResult.id');
    expect(source).toContain('logsSendResult = result');
  });

  it('resolves a failed send through the coded error mapper, never raw text', () => {
    expect(source).toContain('logsUploadErrorText(err)');
    expect(source).not.toMatch(/logsSendFailure\s*=\s*.*err\.message/);
  });

  it('offers the manual download as a fallback after any failure', () => {
    expect(source).toContain("msg('settings.aboutLogsSendFailedHint')");
  });

  it('warns about dropped rotations when the upload had to trim the bundle', () => {
    expect(source).toContain("msg('settings.aboutLogsSendDroppedNote'");
    expect(source).toContain('logsSendResult.dropped');
  });
});
