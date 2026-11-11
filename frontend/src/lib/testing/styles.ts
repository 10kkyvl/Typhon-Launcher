import { compile } from 'svelte/compiler';
import { onTestFinished } from 'vitest';

// For *.dom.test.ts only. The test build drops component CSS, so getComputedStyle sees no rule of a
// mounted component. This puts the component's own stylesheet into the document for the current test,
// scoped with the class Svelte gave `element`, so the cascade can be asserted: which rule wins and
// under which ancestor class. jsdom still lays nothing out, resolves no var() and applies no @media.
export function applyStyles(element: Element, source: string): void {
  const scope = [...element.classList].find((name) => name.startsWith('svelte-'));
  if (!scope) throw new Error('the element carries no Svelte scope class');
  const css = compile(source, { css: 'external', cssHash: () => scope, filename: 'Component.svelte' }).css?.code;
  if (!css) throw new Error('the component has no style block');
  const style = document.createElement('style');
  style.textContent = css;
  document.head.append(style);
  onTestFinished(() => style.remove());
}
