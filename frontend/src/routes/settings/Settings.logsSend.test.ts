import { describe, expect, it } from 'vitest';
import source from './Settings.svelte?raw';

describe('Settings send-logs flow', () => {
  it('never sends on the button click itself, only after confirmation', () => {
    expect(source).toMatch(/onclick=\{openSendLogsConfirm\}/);
    expect(source).not.toMatch(/onclick=\{[^}]*sendLogs\(\)[^}]*\}/);
  });

  it('routes the confirmed send through ConfirmModal', () => {
    expect(source).toMatch(/\{#if sendLogsConfirmOpen\}/);
    expect(source).toContain('<ConfirmModal');
    expect(source).toContain('prompt={sendLogsPrompt()}');
    expect(source).toContain('onconfirm={confirmSendLogs}');
  });

  it('only calls sendLogs from inside the confirmed handler', () => {
    expect(source).toMatch(/async function confirmSendLogs\(\)[\s\S]*?await sendLogs\(\)/);
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
