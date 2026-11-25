import { afterEach, describe, expect, it } from 'vitest';
import { leaveTarget, placeOf } from './rowFocus';

// The screen has no control of a section outside its rows today; the rule's step for one is pinned
// here, on the screen's own rule, so a control added to a section header later gets focus first.
function page(sectionControl: boolean) {
  document.body.innerHTML =
    '<h1 tabindex="-1">LAN</h1>' +
    '<section data-lan-section="sharing">' +
    (sectionControl ? '<button id="section-action">Section action</button>' : '') +
    '<ul><li data-lan-row="g1"><button data-row-control="stop" id="stop">Stop</button></li></ul>' +
    '</section>' +
    '<section data-lan-section="transfers"><ul><li data-lan-row="t1"><button data-row-control="cancel">Cancel</button></li></ul></section>';
}

afterEach(() => {
  document.body.replaceChildren();
});

describe('LAN focus rule: the only row of a section leaves', () => {
  it('focus goes to the section’s first control when it has one', () => {
    page(true);
    const place = placeOf(document.getElementById('stop'));
    document.querySelector('[data-lan-row="g1"]')?.remove();

    expect(leaveTarget(place!)).toBe(document.getElementById('section-action'));
  });

  it('else to the page heading, never into another section', () => {
    page(false);
    const place = placeOf(document.getElementById('stop'));
    document.querySelector('[data-lan-row="g1"]')?.remove();

    expect(leaveTarget(place!)).toBe(document.querySelector('h1'));
  });
});
