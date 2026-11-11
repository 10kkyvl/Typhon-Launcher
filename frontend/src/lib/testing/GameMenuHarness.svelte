<script lang="ts">
  import GameQuickMenu from '../components/GameQuickMenu.svelte';
  import { openGameMenu } from '../stores/gameMenu';

  // A game card with its context menu, as on the library page. For GameQuickMenu.dom.test.ts.
</script>

<div class="card" role="presentation" oncontextmenu={(event) => openGameMenu(event, 'g1')}>
  <a href="#g1">Hades</a>
</div>
<GameQuickMenu />
