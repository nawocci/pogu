<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Busy from '../components/Busy.svelte';
  import { api, type Group } from '../api';
  import { toast } from '../toast.svelte';

  let { group, onclose, onsaved }: { group?: Group; onclose: () => void; onsaved: () => void } = $props();

  /* svelte-ignore state_referenced_locally */
  let name = $state(group?.name ?? '');
  /* svelte-ignore state_referenced_locally */
  const editing = group !== undefined && group !== null;
  let busy = $state(false);
  let error = $state('');

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (!name.trim()) {
      error = 'Group name is required.';
      return;
    }
    busy = true;
    try {
      const input = { name: name.trim(), selection: group?.selection ?? 'first', enabled: group?.enabled ?? true };
      if (editing && group) {
        await api.updateGroup(group.id, input);
        toast('success', `Group "${input.name}" updated`);
      } else {
        const created = await api.createGroup(input);
        toast('success', `Group "${created.name}" created`, {
          action: { label: 'Open', href: `/groups/${created.id}` },
        });
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

<Modal title={editing ? 'Edit group' : 'Add group'} {onclose}>
  <form novalidate onsubmit={save}>
    <label class="field">
      <span>Name</span>
      <input type="text" bind:value={name} placeholder="frontier, glm-flash, …" />
      {#if name.trim()}
        <p class="hint enter-blip">Clients call this as <code class="font-mono text-xs text-accent-ink">"model": "{name.trim()}"</code></p>
      {/if}
    </label>
    {#if error}<p class="error-line enter-blip">{error}</p>{/if}
    <div class="modal-actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary" disabled={busy} aria-busy={busy}>
        <Busy busy={busy} text={editing ? 'Save changes' : 'Create group'} wide="Save changes" />
      </button>
    </div>
  </form>
</Modal>
