<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Busy from '../components/Busy.svelte';
  import { api, type ProviderKey } from '../api';
  import { toast } from '../toast.svelte';

  let { providerId, key, onclose, onsaved }: { providerId: number; key: ProviderKey; onclose: () => void; onsaved: () => void } = $props();

  /* svelte-ignore state_referenced_locally */
  let name = $state(key.name);
  /* svelte-ignore state_referenced_locally */
  let enabled = $state(key.enabled);
  let busy = $state(false);
  let error = $state('');

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (!name.trim()) {
      error = 'Name is required.';
      return;
    }
    busy = true;
    try {
      await api.updateProviderKey(providerId, key.id, { name: name.trim(), enabled });
      toast('success', 'Key updated');
      onsaved();
      onclose();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title="Edit API key" {onclose}>
  <form novalidate onsubmit={save}>
    <label class="field">
      <span>Name</span>
      <input type="text" bind:value={name} />
    </label>
    <label class="field">
      <span>Key</span>
      <input type="text" value={key.masked_key} disabled />
      <p class="hint">Key values cannot be changed — delete and re-add to replace one.</p>
    </label>
    <label class="field">
      <span>Enabled</span>
      <span class="flex items-center gap-2 text-[13px] text-secondary">
        <input type="checkbox" bind:checked={enabled} class="size-4" />
        Eligible for request selection
      </span>
    </label>
    {#if error}<p class="error-line enter-blip">{error}</p>{/if}
    <div class="modal-actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary" disabled={busy} aria-busy={busy}>
        <Busy busy={busy} text="Save changes" wide="Save changes" />
      </button>
    </div>
  </form>
</Modal>
