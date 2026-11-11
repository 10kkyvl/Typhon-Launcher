import { render } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { msg } from '../../lib/i18n';
import { describedControls, hintOnFocus } from '../../lib/testing/hints';
import HiddenBadge from './HiddenBadge.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

describe('HiddenBadge', () => {
  it('who the block is hidden from reaches a keyboard and a screen-reader user', async () => {
    render(HiddenBadge, { props: { text: msg('social.statsHiddenFromOthers') } });
    const controls = describedControls(document, msg('social.statsHiddenFromOthers'));
    expect(controls).toHaveLength(1);
    expect(controls[0].textContent?.trim()).toBe(msg('social.hiddenLabel'));
    expect(await hintOnFocus(controls[0])).toBe(msg('social.statsHiddenFromOthers'));
  });
});
