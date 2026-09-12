import { describe, expect, it } from 'vitest';
import { chatTextParts } from './chatText';

describe('chatTextParts', () => {
  it('keeps ordinary text plain and exposes only http links', () => {
    expect(chatTextParts('Привет https://example.com/path, и www.example.com')).toEqual([
      { text: 'Привет ' },
      { text: 'https://example.com/path', href: 'https://example.com/path' },
      { text: ',' },
      { text: ' и www.example.com' },
    ]);
  });

  it('supports multiple links and preserves line breaks', () => {
    expect(chatTextParts('http://a.test\nhttps://b.test')).toEqual([
      { text: 'http://a.test', href: 'http://a.test' },
      { text: '\n' },
      { text: 'https://b.test', href: 'https://b.test' },
    ]);
  });
});
