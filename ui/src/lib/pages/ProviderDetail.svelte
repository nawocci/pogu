<script lang="ts">
  import { api, providerInput, type Provider, type ProviderKey, type Model } from '../api';
  import { prettyDate, providerTypeLabel as typeLabel } from '../format';
  import { store } from '../store.svelte';
  import { navigate } from '../router.svelte';
  import { toast } from '../toast.svelte';
  import { reveal, blipFade } from '../motion.svelte';
  import { createArmed } from '../armed.svelte';
  import Lamp from '../components/Lamp.svelte';
  import Busy from '../components/Busy.svelte';
  import Fit from '../components/Fit.svelte';
  import ProviderModal from './ProviderModal.svelte';
  import ProviderKeyModal from './ProviderKeyModal.svelte';
  import ProviderKeyEditModal from './ProviderKeyEditModal.svelte';
  import BatchImportModal from './BatchImportModal.svelte';
  import ModelModal from './ModelModal.svelte';

  let { providerId }: { providerId: number } = $props();

  let provider = $state<Provider | null>(null);
  let keys = $state<ProviderKey[]>([]);
  let models = $state<Model[]>([]);
  let loading = $state(true);
  let error = $state('');

  let showEdit = $state(false);
  let showAddKey = $state(false);
  let editingKey = $state<ProviderKey | null>(null);
  let showImport = $state(false);
  let showAddModel = $state(false);
  let editingModel = $state<Model | null>(null);

  let busyToggle = $state(false);
  let busyTest = $state(false);
  let busySync = $state(false);
  let busyKeyAction = $state<{ id: number; action: 'primary' | 'test' | 'toggle' } | null>(null);
  let busyModel = $state<number | null>(null);
  const armed = createArmed();

  async function loadDetail() {
    loading = true;
    error = '';
    try {
      const [p, k, allModels] = await Promise.all([api.getProvider(providerId), api.providerKeys(providerId), api.models()]);
      provider = p;
      keys = k ?? [];
      models = (allModels ?? []).filter((m) => m.provider_id === providerId);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void loadDetail();
  });

  function fail(e: unknown) {
    toast('error', e instanceof Error ? e.message : String(e));
  }

  async function setEnabled(enabled: boolean) {
    const current = provider;
    if (!current || busyToggle) return;
    busyToggle = true;
    try {
      provider = await store.mutate(() => api.updateProvider(current.id, providerInput(current, { enabled })));
    } catch (e) {
      fail(e);
    } finally {
      busyToggle = false;
    }
  }

  async function runTest() {
    if (!provider || busyTest) return;
    busyTest = true;
    try {
      const r = await api.testProvider(provider.id);
      if (r?.ok === false) toast('error', r.message ?? 'Provider is unreachable');
      else toast('success', r?.message ?? 'Connection successful');
    } catch (e) {
      fail(e);
    } finally {
      busyTest = false;
    }
  }

  async function runSync() {
    if (!provider || busySync) return;
    busySync = true;
    try {
      const s = await api.syncProvider(provider.id);
      await loadDetail();
      toast('success', `Catalog synced — ${s.free} free models, ${s.added} added, ${s.disabled} disabled`);
    } catch (e) {
      fail(e);
    } finally {
      busySync = false;
    }
  }

  async function testKey(key: ProviderKey) {
    if (busyKeyAction) return;
    busyKeyAction = { id: key.id, action: 'test' };
    try {
      const r = await api.testProviderKey(providerId, key.id);
      if (r?.ok === false) toast('error', r.message ?? 'Key failed');
      else toast('success', r?.message ?? 'Connection successful');
    } catch (e) {
      fail(e);
    } finally {
      busyKeyAction = null;
    }
  }

  async function makePrimary(key: ProviderKey) {
    if (busyKeyAction) return;
    busyKeyAction = { id: key.id, action: 'primary' };
    try {
      keys = await api.makeProviderKeyPrimary(providerId, key.id);
      toast('success', `Key "${key.name}" is now primary`);
    } catch (e) {
      fail(e);
    } finally {
      busyKeyAction = null;
    }
  }

  async function setKeyEnabled(key: ProviderKey, enabled: boolean) {
    if (busyKeyAction) return;
    busyKeyAction = { id: key.id, action: 'toggle' };
    try {
      await api.updateProviderKey(providerId, key.id, { name: key.name, enabled });
      await loadDetail();
    } catch (e) {
      fail(e);
    } finally {
      busyKeyAction = null;
    }
  }

  async function deleteKey(key: ProviderKey) {
    try {
      await api.deleteProviderKey(providerId, key.id);
      await loadDetail();
      toast('success', `Key "${key.name}" deleted`);
    } catch (e) {
      fail(e);
    }
  }

  async function setSelection(mode: 'first' | 'round_robin') {
    const current = provider;
    if (!current || current.key_selection === mode) return;
    try {
      provider = await store.mutate(() =>
        api.updateProvider(current.id, providerInput(current, { key_selection: mode })),
      );
      toast('success', mode === 'round_robin' ? 'Key selection set to round robin' : 'Key selection set to primary first');
    } catch (e) {
      fail(e);
    }
  }

  async function deleteProvider() {
    if (!provider) return;
    const id = provider.id;
    try {
      const name = provider.name;
      await api.deleteProvider(id);
      await store.refresh();
      toast('success', `Provider "${name}" deleted`);
      navigate('/providers', { replace: true });
    } catch (e) {
      fail(e);
    }
  }

  async function removeModel(m: Model) {
    try {
      await api.deleteModel(m.id);
      await loadDetail();
      toast('success', `Model "${m.name}" deleted`);
    } catch (e) {
      fail(e);
    }
  }

  async function toggleModel(m: Model) {
    if (busyModel) return;
    busyModel = m.id;
    try {
      await api.updateModel(m.id, { provider_id: m.provider_id, name: m.name, enabled: !m.enabled });
      await loadDetail();
    } catch (e) {
      fail(e);
    } finally {
      busyModel = null;
    }
  }
</script>

{#if loading && !provider}
  <p class="flex items-center gap-3 py-10 text-sm text-tertiary"><Lamp state="live" /> Loading…</p>
{:else if error && !provider}
  <p class="error-line">{error}</p>
{:else if !provider}
  <div class="empty">
    <h2 class="mb-2">Provider not found</h2>
    <p class="mt-2 mb-[18px] text-tertiary">It may have been deleted.</p>
    <a class="btn" href="/providers">Back to providers</a>
  </div>
{:else}
  <a
    use:reveal={{ kind: 'blip' }}
    class="back-link"
    href="/providers"
  >← Providers</a>

  <header use:reveal={{ kind: 'rise', i: 1 }} class="mt-4 mb-8 flex flex-wrap items-start justify-between gap-5">
    <div>
      <h1 class="leading-none">{provider.name}</h1>
      <p class="mt-2.5 flex flex-wrap items-center gap-2.5 text-[13px] text-tertiary">
        <code class="rounded-full bg-accent-dim px-2 py-0.5 font-mono text-[10.5px] font-medium text-accent-ink">{provider.prefix}</code>
        {#if provider.builtin}
          <span class="rounded-full border border-line px-2 py-px font-mono text-[10px] font-medium tracking-[0.04em] uppercase" title="Built-in provider — permanent, no API key required">Built-in</span>
        {/if}
        <span>{typeLabel(provider.type)}</span>
        <span class="text-muted">·</span>
        <code class="font-mono text-[11.5px] text-primary [overflow-wrap:anywhere]">{provider.base_url}</code>
      </p>
    </div>
    <div use:reveal={{ kind: 'pop', i: 2 }} class="flex flex-wrap items-center gap-2">
      <Lamp state={provider.enabled ? 'on' : 'off'} label={provider.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" />
      <button class="btn btn-sm" onclick={runTest} disabled={busyTest} aria-busy={busyTest}>
        <Busy busy={busyTest} text={busyTest ? 'Testing…' : 'Test connection'} wide="Test connection" />
      </button>
      {#if provider.builtin}
        <button class="btn btn-sm" onclick={runSync} disabled={busySync} aria-busy={busySync}>
          <Busy busy={busySync} text={busySync ? 'Syncing…' : 'Sync models'} wide="Sync models" />
        </button>
      {/if}
      <button class="btn btn-sm" onclick={() => provider && setEnabled(!provider.enabled)} disabled={busyToggle} aria-busy={busyToggle}>
        <Busy busy={busyToggle} text={provider.enabled ? 'Disable' : 'Enable'} wide="Disable" />
      </button>
      {#if !provider.builtin}
        <button class="btn btn-sm" onclick={() => (showEdit = true)}>Edit</button>
        <button
          class="btn btn-sm btn-danger"
          class:btn-armed={armed.is('provider')}
          onclick={() => armed.confirm('provider', deleteProvider)}
        >
          <Fit text={armed.is('provider') ? 'Confirm delete' : 'Delete'} wide="Confirm delete" />
        </button>
      {/if}
    </div>
  </header>

  {#if !provider.enabled}
    <p
      class="mb-7 rounded-sm border border-error-soft bg-error-soft px-3.5 py-3 text-[13px] text-error-muted"
      transition:blipFade={ { duration: 260 } }
    >
      This provider is disabled. Inference requests will not route to it until you enable it.
    </p>
  {/if}

  <section use:reveal={{ kind: 'rise', i: 2 }} aria-labelledby="keys-h">
    <div class="sec-head">
      <h2 id="keys-h">API keys <span class="ml-[5px] font-mono text-[13px] text-accent-ink [vertical-align:3px]">{keys.length}</span></h2>
      {#if !provider.builtin}
        <div class="flex gap-2">
          <button class="btn btn-sm" onclick={() => (showImport = true)}>Batch import</button>
          <button class="btn btn-sm btn-primary" onclick={() => (showAddKey = true)}>Add key</button>
        </div>
      {/if}
    </div>

    {#if provider.builtin}
      <p class="text-[13px] text-tertiary">No API key required — pogu manages the public free-tier credential automatically. Disable the provider to stop all traffic to this upstream.</p>
    {:else}

    <div class="mb-4 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-sm border border-line bg-raised px-3.5 py-2.5">
      <span class="font-mono text-[10px] font-semibold tracking-[0.07em] text-tertiary uppercase">Key selection</span>
      <div class="flex gap-0.5 rounded-sm border border-line bg-well p-0.5" role="group" aria-label="Key selection strategy">
        <button
          aria-pressed={provider.key_selection === 'first'}
          class="rounded-[2px] px-3 py-1 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {provider.key_selection === 'first'
            ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
            : 'text-tertiary hover:text-primary'}"
          onclick={() => setSelection('first')}
        >Primary first</button>
        <button
          aria-pressed={provider.key_selection === 'round_robin'}
          class="rounded-[2px] px-3 py-1 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {provider.key_selection === 'round_robin'
            ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
            : 'text-tertiary hover:text-primary'}"
          onclick={() => setSelection('round_robin')}
        >Round robin</button>
      </div>
      <p class="m-0 text-[12.5px] text-tertiary">
        {provider.key_selection === 'round_robin'
          ? 'Keys rotate per request; failed keys are skipped.'
          : 'Requests always start at the primary key and fail over in order.'}
      </p>
    </div>

    {#if keys.length === 0}
      <p class="text-[13px] text-tertiary">No keys yet — requests to this provider will fail without at least one enabled key.</p>
    {:else}
      <table class="table-data table-cards">
        <thead>
          <tr><th>Order</th><th>Name</th><th>Key</th><th>Status</th><th>Last used</th><th>Created</th><th></th></tr>
        </thead>
        <tbody>
          {#each keys as k, i (k.id)}
            {@const armedNow = armed.is('k' + k.id)}
            <tr>
              <td data-label="Order">
                {#if i === 0}
                  <span class="inline-flex items-center gap-[6px] rounded-full border border-success-soft bg-success-soft px-1.5 py-px font-mono text-[10px] tracking-[0.07em] text-success-muted uppercase">Primary</span>
                {:else}
                  <span class="font-mono text-[11.5px] text-tertiary">#{i + 1}</span>
                {/if}
              </td>
              <td data-label="Name" class="font-medium text-paper"><span class="block max-w-[220px] truncate">{k.name}</span></td>
              <td data-label="Key"><code class="font-mono text-[12.5px] text-primary">{k.masked_key}</code></td>
              <td data-label="Status"><Lamp state={k.enabled ? 'on' : 'off'} label={k.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" /></td>
              <td data-label="Last used"><time class="font-mono text-[11px] text-tertiary">{prettyDate(k.last_used_at)}</time></td>
              <td data-label="Created"><time class="font-mono text-[11px] text-tertiary">{prettyDate(k.created_at)}</time></td>
              <td class="cell-actions text-right whitespace-nowrap">
                {#if i > 0}
                  <button class="linkish" onclick={() => makePrimary(k)} disabled={!!busyKeyAction} aria-busy={busyKeyAction?.id === k.id && busyKeyAction?.action === 'primary'}>
                    <Busy busy={busyKeyAction?.id === k.id && busyKeyAction?.action === 'primary'} text="Primary" />
                  </button>
                {/if}
                <button class="linkish" onclick={() => testKey(k)} disabled={!!busyKeyAction} aria-busy={busyKeyAction?.id === k.id && busyKeyAction?.action === 'test'}>
                  <Busy busy={busyKeyAction?.id === k.id && busyKeyAction?.action === 'test'} text="Test" />
                </button>
                <button class="linkish" onclick={() => (editingKey = k)}>Edit</button>
                <button class="linkish" onclick={() => setKeyEnabled(k, !k.enabled)} disabled={!!busyKeyAction}>
                  <Busy busy={busyKeyAction?.id === k.id && busyKeyAction?.action === 'toggle'} text={k.enabled ? 'Disable' : 'Enable'} wide="Disable" />
                </button>
                <button class="linkish linkish-del" onclick={() => armed.confirm('k' + k.id, () => deleteKey(k))}>
                  <Fit text={armedNow ? 'Confirm?' : 'Delete'} wide="Confirm?" />
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
    {/if}
  </section>

  <section use:reveal={{ kind: 'rise', i: 3 }} aria-labelledby="models-h" class="mt-[42px]">
    <div class="sec-head">
      <h2 id="models-h">Models <span class="ml-[5px] font-mono text-[13px] text-accent-ink [vertical-align:3px]">{models.length}</span></h2>
      {#if provider.builtin}
        <p class="m-0 text-[12.5px] text-tertiary">Managed by catalog sync — use Sync models above to refresh.</p>
      {:else}
        <button class="btn btn-sm btn-primary" onclick={() => { editingModel = null; showAddModel = true; }}>Add model</button>
      {/if}
    </div>
    {#if models.length === 0}
      <p class="text-[13px] text-tertiary">
        {#if provider.builtin}
          No models yet. {#if provider.enabled}Sync models to fetch the current free catalog.{:else}Enable the provider, then sync models.{/if}
        {:else}
          No models yet. Add the upstream model identifiers clients should reach as <code class="font-mono text-xs">{provider.prefix}/&lt;name&gt;</code>.
        {/if}
      </p>
    {:else}
      <table class="table-data table-cards">
        <thead>
          <tr><th>Model</th><th>Public ID</th><th>Status</th><th>Updated</th><th></th></tr>
        </thead>
        <tbody>
          {#each models as m (m.id)}
            {@const armedNow = armed.is('m' + m.id)}
            <tr>
              <td data-label="Model" class="font-medium text-paper">{m.name}</td>
              <td data-label="Public ID"><code class="font-mono text-[12.5px] text-primary">{m.public_id}</code></td>
              <td data-label="Status"><Lamp state={m.enabled ? 'on' : 'off'} label={m.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" /></td>
              <td data-label="Updated"><time class="font-mono text-[11px] text-tertiary">{prettyDate(m.updated_at)}</time></td>
              <td class="cell-actions text-right whitespace-nowrap">
                <button class="linkish" onclick={() => toggleModel(m)} disabled={!!busyModel} aria-busy={busyModel === m.id}>
                  <Busy busy={busyModel === m.id} text={m.enabled ? 'Disable' : 'Enable'} wide="Disable" />
                </button>
                {#if !provider.builtin}
                  <button class="linkish" onclick={() => { editingModel = m; showAddModel = true; }}>Edit</button>
                  <button class="linkish linkish-del" onclick={() => armed.confirm('m' + m.id, () => removeModel(m))}>
                    <Fit text={armedNow ? 'Confirm?' : 'Delete'} wide="Confirm?" />
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
{/if}

{#if provider && showEdit}
  <ProviderModal existing={provider} onclose={() => (showEdit = false)} />
{/if}
{#if provider && showAddKey}
  <ProviderKeyModal providerId={provider.id} onclose={() => (showAddKey = false)} onsaved={loadDetail} />
{/if}
{#if provider && editingKey}
  <ProviderKeyEditModal
    providerId={provider.id}
    key={editingKey}
    onclose={() => (editingKey = null)}
    onsaved={loadDetail}
  />
{/if}
{#if provider && showImport}
  <BatchImportModal providerId={provider.id} onclose={() => (showImport = false)} onsaved={loadDetail} />
{/if}
{#if provider && showAddModel}
  <ModelModal
    providerId={provider.id}
    providerPrefix={provider.prefix}
    model={editingModel}
    onclose={() => (showAddModel = false)}
    onsaved={loadDetail}
  />
{/if}
