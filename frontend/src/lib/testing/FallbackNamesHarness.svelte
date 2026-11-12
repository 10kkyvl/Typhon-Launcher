<script lang="ts">
  import Artwork from '../components/Artwork.svelte';
  import Avatar from '../components/Avatar.svelte';

  // Controls that hold a cover or an avatar with no picture to show, as the pages build them: a row
  // named by its text, a control whose picture alone names it, and one named by its own label.
  // For fallbackNames.dom.test.ts.
</script>

<button type="button" data-testid="game-row"><Artwork src="" alt="" label="Celeste" /><span>Celeste (2018)</span></button>
<button type="button" data-testid="cover-only"><Artwork src="" alt="Celeste" /></button>
<a href="#hades" data-testid="cover-link"><Artwork src="/hades.png" alt="Hades" /></a>
<div data-testid="bare"><Artwork src="" alt="" label="Hollow Knight" /></div>

<button type="button" data-testid="friend-row"><Avatar name="Ann Lee" /><span>Ann Lee</span></button>
<button type="button" data-testid="friend-labelled" aria-label="Profile of Ann Lee"><Avatar name="Ann Lee" /></button>
