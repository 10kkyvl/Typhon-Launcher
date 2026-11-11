import type { Attachment } from 'svelte/attachments';

/**
 * `{@attach portal}` moves the element to the end of `<body>`, out of the app root (`#app`): fixed
 * layers escape transformed, clipped, contained or click-through ancestors (the chat dock is
 * `pointer-events: none`) and stack on the window's z-index scale (shell.css) above whatever is
 * shown, Big Picture included. The overlay window runs the same bundle in its own document, so it
 * gets its own body.
 *
 * The element must be the only root node of its block (`{#if}`, `{#each}` item): Svelte removes a
 * block by walking the siblings between its first and last node, and a moved node has none here.
 * Svelte still owns the node, so its bindings, events and transitions keep working.
 */
export const portal: Attachment<HTMLElement> = (node) => {
  document.body.appendChild(node);
  return () => node.remove();
};
