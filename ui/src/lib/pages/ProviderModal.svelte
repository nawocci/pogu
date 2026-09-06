<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Busy from '../components/Busy.svelte';
  import Dropdown from '../components/Dropdown.svelte';
  import { api, type Provider } from '../api';
  import { store } from '../store.svelte';
  import { toast } from '../toast.svelte';

  /* fields seed once; the modal remounts per open */
  /* svelte-ignore state_referenced_locally */
  let { existing, onclose }: { existing?: Provider; onclose: () => void } = $props();

  /* svelte-ignore state_referenced_locally */
  let name = $state(existing?.name ?? '');
  /* svelte-ignore state_referenced_locally */
  let type = $state<'openai' | 'anthropic'>(existing?.type === 'anthropic' ? 'anthropic' : 'openai');
  /* svelte-ignore state_referenced_locally */
  let prefix = $state(existing?.prefix ?? '');
  /* svelte-ignore state_referenced_locally */
  let baseUrl = $state(existing?.base_url ?? '');
  let apiKey = $state('');
  let busy = $state(false);
  let error = $state('');

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    const input = {
      name: name.trim(),
      type,
      prefix: prefix.trim(),
      base_url: baseUrl.trim().replace(/\/+$/, ''),
      key_selection: 'first' as const,
      enabled: existing?.enabled ?? true,
    };
    if (!input.name) {
      error = 'Name is required.';
      return;
    }
    if (!input.prefix) {
      error = 'Prefix is required — requests address models as prefix/model.';
      return;
    }
    if (!input.base_url) {
      error = 'Base URL is required — for example https://api.openai.com/v1';
      return;
    }
    busy = true;
    try {
      if (existing) {
        await store.mutate(() => api.updateProvider(existing.id, input));
        toast('success', `Provider "${input.name}" updated`);
      } else {
        const payload = apiKey.trim() ? { ...input, api_key: apiKey.trim() } : input;
        const created = await store.mutate(() => api.createProvider(payload));
        toast('success', `Provider "${created.name}" created`, {
          action: { label: 'Open', href: `/providers/${created.id}` },
        });
      }
      onclose();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={existing ? 'Edit provider' : 'Add provider'} {onclose}>
  <form novalidate onsubmit={save}>
    <div class="grid grid-cols-2 gap-4 max-compact:grid-cols-1">
      <label class="field">
        <span>Name</span>
        <input type="text" bind:value={name} placeholder="OpenRouter" />
      </label>
      <label class="field">
        <span>Upstream API</span>
        <Dropdown
          value={type}
          onchange={(v) => (type = v as 'openai' | 'anthropic')}
          options={[
            { value: 'openai', label: 'OpenAI' },
            { value: 'anthropic', label: 'Anthropic' },
          ]}
        />
      </label>
    </div>
    <label class="field">
      <span>Prefix</span>
      <input type="text" bind:value={prefix} placeholder="or" />
      <p class="hint">Models are addressed as <code class="font-mono text-xs">prefix/model</code>.</p>
    </label>
    <label class="field">
      <span>Base URL</span>
      <input type="text" bind:value={baseUrl} placeholder="https://api.openai.com/v1" />
      <p class="hint">API root including the version segment.</p>
    </label>
    {#if !existing}
      <label class="field">
        <span>Initial API key — optional</span>
        <input type="password" bind:value={apiKey} placeholder="sk-…" autocomplete="new-password" />
        <p class="hint">More keys — with failover — can be added on the provider's page.</p>
      </label>
    {/if}

    {#if error}<p class="error-line enter-blip">{error}</p>{/if}

    <div class="modal-actions mt-5">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary" disabled={busy} aria-busy={busy}>
        <Busy busy={busy} text={existing ? 'Save changes' : 'Add provider'} wide="Save changes" />
      </button>
    </div>
  </form>
</Modal>
