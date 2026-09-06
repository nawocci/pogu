<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Busy from '../components/Busy.svelte';
  import { api, type Model } from '../api';
  import { toast } from '../toast.svelte';

  let {
    providerId,
    providerPrefix,
    model,
    onclose,
    onsaved,
  }: { providerId: number; providerPrefix: string; model: Model | null; onclose: () => void; onsaved: () => void } = $props();

  /* svelte-ignore state_referenced_locally */
  let name = $state(model?.name ?? '');
  let busy = $state(false);
  let error = $state('');

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (!name.trim()) {
      error = 'Model identifier is required.';
      return;
    }
    busy = true;
    try {
      const input = { provider_id: providerId, name: name.trim(), enabled: model?.enabled ?? true };
      if (model) {
        await api.updateModel(model.id, input);
        toast('success', `Model "${input.name}" updated`);
      } else {
        await api.createModel(input);
        toast('success', `Model "${input.name}" added`);
      }
      onsaved();
      onclose();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={model ? 'Edit model' : 'Add model'} {onclose}>
  <form novalidate onsubmit={save}>
    <label class="field">
      <span>Upstream model identifier</span>
      <input type="text" bind:value={name} placeholder="mimo-v2.5-pro, gpt-4o, etc." />
      {#if name.trim()}
        <p class="hint enter-blip">Public route: <code class="font-mono text-xs text-accent-ink">{providerPrefix}/{name.trim()}</code></p>
      {/if}
    </label>
    {#if error}<p class="error-line enter-blip">{error}</p>{/if}
    <div class="modal-actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary" disabled={busy} aria-busy={busy}>
        <Busy busy={busy} text={model ? 'Save changes' : 'Add model'} wide="Save changes" />
      </button>
    </div>
  </form>
</Modal>
