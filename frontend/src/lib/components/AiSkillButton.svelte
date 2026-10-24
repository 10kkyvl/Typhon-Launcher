<script lang="ts">
  import Button from './Button.svelte';
  import { msg } from '../i18n';
  import { openFolder } from '../services/settings';
  import { saveAISkill, selectSkillFolder } from '../services/theme';
  import { toast } from '../stores/toasts';
  import { themeErrorText } from '../theme/themeErrors';

  interface Props {
    label: string;
  }

  let { label }: Props = $props();
  let saving = $state(false);

  async function run() {
    saving = true;
    try {
      const dir = await selectSkillFolder();
      if (!dir) return;
      const path = await saveAISkill(dir);
      toast(msg('settings.aiSkillSavedToast', { path }), 'success', {
        label: msg('settings.aiSkillOpenFolder'),
        run: () => {
          openFolder(path).catch((err) => toast(themeErrorText(err), 'danger'));
        },
      });
    } catch (err) {
      toast(themeErrorText(err), 'danger');
    } finally {
      saving = false;
    }
  }
</script>

<Button size="sm" disabled={saving} onclick={run}>{saving ? msg('settings.aiSkillSaving') : label}</Button>
