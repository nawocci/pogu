<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Copy from '../components/Copy.svelte';
  import Busy from '../components/Busy.svelte';
  import { api } from '../api';
  import { toast } from '../toast.svelte';

  let { providerId, onclose, onsaved }: { providerId: number; onclose: () => void; onsaved: () => void } = $props();

  let name = $state('');
  let secret = $state('');
  let busy = $state(false);
  let error = $state('');
  let created = $state('');

  async function create(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (!secret.trim()) {
      error = 'The key value is required.';
      return;
    }
    busy = true;
    try {
      const input: { secret: string; name?: string } = { secret: secret.trim() };
      if (name.trim()) input.name = name.trim();
      const r = await api.createProviderKey(providerId, input);
      created = r.secret;
      toast('success', `Key "${r.key.name}" added`);
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={created ? 'API key created' : 'Add API key'} {onclose}>
  {#if !created}
    <form novalidate onsubmit={create}>
      <label class="field">
        <span>Name — optional</span>
        <input type="text" bind:value={name} placeholder="Work, Backup, etc." />
      </label>
      <label class="field">
        <span>Key</span>
        <input type="password" bind:value={secret} placeholder="sk-…" autocomplete="new-password" />
      </label>
      <p class="mt-4 mb-4 rounded-sm border border-line-soft bg-well px-3.5 py-3 text-[13px] text-secondary">
        pogu encrypts the key at rest. It is shown once, below, after creation and cannot be retrieved again.
      </p>
      {#if error}<p class="error-line enter-blip">{error}</p>{/if}
      <div class="modal-actions">
        <button type="button" class="btn" onclick={onclose}>Cancel</button>
        <button type="submit" class="btn btn-primary" disabled={busy} aria-busy={busy}>
          <Busy busy={busy} text="Add key" wide="Add key" />
        </button>
      </div>
    </form>
  {:else}
    <p class="mb-4 text-[13px] text-tertiary">
      Copy it now and store it somewhere safe — <b class="font-semibold text-warning-muted">this key will only be shown once</b>.
      It is stored encrypted and cannot be revealed again.
    </p>
    <div class="panel mb-4 flex items-center gap-2 px-3.5 py-3">
      <code class="min-w-0 flex-1 font-mono text-[13px] break-all text-accent-ink select-all">{created}</code>
      <Copy value={created} label="Copy key" silent />
    </div>
    <div class="modal-actions">
      <button
        class="btn btn-primary"
        onclick={() => {
          onsaved();
          onclose();
        }}
      >Done</button>
    </div>
  {/if}
</Modal>
