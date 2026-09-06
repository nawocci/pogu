<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Busy from '../components/Busy.svelte';
  import { api } from '../api';

  let { onclose, oncreated }: { onclose: () => void; oncreated: (result: { secret: string }) => void } = $props();

  let name = $state('');
  let busy = $state(false);
  let error = $state('');

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    if (!name.trim()) {
      error = 'Name is required — it labels the key in lists and telemetry.';
      return;
    }
    busy = true;
    try {
      const result = await api.createKey(name.trim());
      oncreated(result);
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title="Create API key" {onclose}>
  <form novalidate onsubmit={submit}>
    <label class="field">
      <span>Name</span>
      <input type="text" bind:value={name} placeholder="My application" />
      <p class="hint">A label so you can tell keys apart — e.g. which app uses them.</p>
    </label>
    <p class="mt-4 mb-4 rounded-sm border border-line-soft bg-well px-3.5 py-3 text-[13px] text-secondary">
      pogu generates the secret for you as <code class="font-mono text-xs text-accent-ink">sk-pogu-…</code>.
      It is shown once after creation and stored only as a hash.
    </p>
    {#if error}<p class="error-line enter-blip">{error}</p>{/if}
    <div class="modal-actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary" disabled={busy} aria-busy={busy}>
        <Busy busy={busy} text="Generate key" wide="Generate key" />
      </button>
    </div>
  </form>
</Modal>
