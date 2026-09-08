import { describe, expect, it } from 'vitest';
import {
  blockPrompt,
  cancelDownloadPrompt,
  clearHistoryPrompt,
  deleteThemePrompt,
  discardDownloadPrompt,
  forgetSyncPrompt,
  removeDownloadPrompt,
  removeSourcePrompt,
  resetAppearancePrompt,
  unfriendPrompt,
} from './prompts';

describe('unfriendPrompt', () => {
  it('names the friend and warns that a new request is needed', () => {
    const prompt = unfriendPrompt('Аня');
    expect(prompt.title).toBe('Удалить из друзей');
    expect(prompt.text).toContain('Аня');
    expect(prompt.text).toContain('заявка');
    expect(prompt.confirm).toBe('Удалить из друзей');
  });
});

describe('blockPrompt', () => {
  it('warns a friendship is lost when the target is a friend', () => {
    const prompt = blockPrompt('Аня', true);
    expect(prompt.title).toBe('Заблокировать');
    expect(prompt.text).toContain('Аня');
    expect(prompt.note).toContain('друзьями');
  });

  it('leaves out the friendship warning for a stranger', () => {
    expect(blockPrompt('Аня', false).note).toBeUndefined();
  });
});

describe('download prompts', () => {
  it('tells removal keeps the files on disk', () => {
    const prompt = removeDownloadPrompt('Portal 2');
    expect(prompt.text).toContain('Portal 2');
    expect(prompt.text).toContain('останутся на диске');
  });

  it('tells discarding erases the files', () => {
    const prompt = discardDownloadPrompt('Portal 2');
    expect(prompt.text).toContain('Portal 2');
    expect(prompt.text).toContain('заново');
    expect(prompt.confirm).toBe('Удалить загрузку и файлы');
  });

  it('keeps the cancel wording of a running download', () => {
    const prompt = cancelDownloadPrompt('Portal 2');
    expect(prompt.text).toContain('Portal 2');
    expect(prompt.cancel).toBe('Не отменять');
    expect(prompt.confirm).toBe('Отменить загрузку');
  });
});

describe('settings prompts', () => {
  it('names the source being removed', () => {
    expect(removeSourcePrompt('Hydra').text).toContain('Hydra');
    expect(removeSourcePrompt('Hydra').title).toBe('Удалить источник');
  });

  it('names the theme being deleted', () => {
    expect(deleteThemePrompt('Ночь').text).toContain('Ночь');
    expect(deleteThemePrompt('Ночь').title).toBe('Удалить тему');
    expect(deleteThemePrompt('Ночь').confirm).toBe('Удалить');
  });

  it('calls the server wipe irreversible', () => {
    const prompt = forgetSyncPrompt();
    expect(prompt.text).toContain('необратимо');
    expect(prompt.busy).toBe('Удаляем…');
  });

  it('offers the appearance reset', () => {
    expect(resetAppearancePrompt().confirm).toBe('Сбросить');
  });

  it('offers the history clear with its progress label', () => {
    const prompt = clearHistoryPrompt();
    expect(prompt.title).toBe('Очистить историю');
    expect(prompt.busy).toBe('Очищаем...');
  });
});
