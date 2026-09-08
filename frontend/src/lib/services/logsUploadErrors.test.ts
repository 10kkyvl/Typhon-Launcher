import { describe, expect, it } from 'vitest';
import { logsUploadErrorText } from './logsUploadErrors';

describe('logsUploadErrorText', () => {
  it('maps the too-large code to its own message', () => {
    expect(logsUploadErrorText(new Error('typhon:diagnostics.log_upload_too_large: 413 body'))).toBe(
      'Логи слишком большие даже после сжатия — сервер не принял архив.',
    );
  });

  it('maps the rate-limited code to its own message', () => {
    expect(logsUploadErrorText(new Error('typhon:diagnostics.log_upload_rate_limited: 429'))).toBe(
      'Слишком много отправок подряд. Подождите немного и попробуйте ещё раз.',
    );
  });

  it('maps the network code to its own message', () => {
    expect(logsUploadErrorText(new Error('typhon:diagnostics.log_upload_network: dial tcp: refused'))).toBe(
      'Не удалось связаться с сервером. Проверьте подключение к интернету.',
    );
  });

  it('maps the generic failed code to its own message', () => {
    expect(logsUploadErrorText(new Error('typhon:diagnostics.log_upload_failed: unexpected status 500'))).toBe(
      'Сервер отказал в приёме логов.',
    );
  });

  it('falls back instead of showing raw go error text', () => {
    expect(logsUploadErrorText(new Error('some unrelated internal error'))).toBe('Не удалось отправить логи.');
    expect(logsUploadErrorText(undefined)).toBe('Не удалось отправить логи.');
  });
});
