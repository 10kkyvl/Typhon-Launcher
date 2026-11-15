<script lang="ts">
  import { gameScope } from '../stores/gameTone';

  let { id }: { id: string | null } = $props();
</script>

<div data-testid="scope" {@attach gameScope(id)}></div>
