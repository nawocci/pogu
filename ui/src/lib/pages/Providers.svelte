<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { api, providerInput, type Provider } from '../api';
  import { providerTypeLabel as typeLabel, keySelectionLabel as selectionLabel } from '../format';
  import { store } from '../store.svelte';
  import { toast } from '../toast.svelte';
  import { reveal } from '../motion.svelte';
  import { createArmed } from '../armed.svelte';
  import Lamp from '../components/Lamp.svelte';
  import ProviderModal from './ProviderModal.svelte';
  import Busy from '../components/Busy.svelte';
  import Fit from '../components/Fit.svelte';

  let showCreate = $state(false);
  const armed = createArmed();
  const busyToggles = new SvelteSet<number>();

  const sorted = $derived([...store.providers].sort((a, b) => a.name.localeCompare(b.name)));

  async function setEnabled(p: (typeof sorted)[number], enabled: boolean) {
    if (busyToggles.has(p.id)) return;
    busyToggles.add(p.id);
    try {
      await store.mutate(() => api.updateProvider(p.id, providerInput(p, { enabled })));
    } catch (e) {
      toast('error', e instanceof Error ? e.message : String(e));
    } finally {
      busyToggles.delete(p.id);
    }
  }

  async function remove(p: Provider) {
    try {
      await store.mutate(() => api.deleteProvider(p.id));
      toast('success', `Provider "${p.name}" deleted`);
    } catch (e) {
      toast('error', e instanceof Error ? e.message : String(e));
    }
  }
</script>

<header use:reveal={{ kind: 'rise', i: 0 }} class="mb-8 flex items-end justify-between gap-5">
  <div>
    <p class="mb-4 font-mono text-[11px] tracking-[0.1em] text-accent-ink uppercase">Management</p>
    <h1 class="leading-none">Providers</h1>
    <p class="mt-2.5 text-sm text-tertiary">Endpoints pogu routes requests through.</p>
  </div>
  <button use:reveal={{ kind: 'pop', i: 1 }} class="btn btn-primary" onclick={() => (showCreate = true)}>Add provider</button>
</header>

{#if store.error}
  <p use:reveal={{ kind: 'blip' }} class="error-line">{store.error}</p>
{/if}

{#if !store.loaded}
  <div class="flex items-center gap-3 py-10 text-sm text-tertiary"><Lamp state="live" /> Loading providers…</div>
{:else if sorted.length === 0}
  <div use:reveal={{ kind: 'blip' }} class="empty">
    <h2 class="mb-2">No providers yet</h2>
    <p class="mt-2 mb-4 text-tertiary">Add your first upstream endpoint to start routing traffic.</p>
    <button class="btn btn-primary" onclick={() => (showCreate = true)}>Add provider</button>
  </div>
{:else}
  <ol class="m-0 flex list-none flex-col gap-2.5 p-0" aria-label="Configured providers">
    {#each sorted as p (p.id)}
      <li use:reveal={{ kind: 'blip', i: 1 }}>
        <div class="card-row">
          <a class="card-hit" href={'/providers/' + p.id} aria-label={p.name}></a>
          <span class="flex min-w-0 flex-col gap-1">
            <span class="inline-flex min-w-0 items-center gap-2">
              <span class="overflow-hidden font-heading text-xl leading-[1.2] font-semibold tracking-[-0.01em] text-ellipsis whitespace-nowrap text-paper">{p.name}</span>
              <code
                class="shrink-0 rounded-full border border-accent-dim bg-accent-dim px-2 py-px font-mono text-[10px] font-medium tracking-[0.04em] text-accent-ink"
                title="Route prefix — {p.prefix}/&lt;model&gt;"
              >{p.prefix}</code>
              {#if p.builtin}
                <span
                  class="shrink-0 rounded-full border border-line px-2 py-px font-mono text-[10px] font-medium tracking-[0.04em] text-tertiary uppercase"
                  title="Built-in provider — permanent, no API key required"
                >Built-in</span>
              {/if}
            </span>
            <code class="truncate font-mono text-[11.5px] leading-[1.4] text-tertiary">{p.base_url}</code>
          </span>
          <span class="flex gap-[18px] font-mono text-[11.5px] whitespace-nowrap text-tertiary max-wide:hidden">
            <span title="Native upstream API"><b class="font-semibold text-paper">{typeLabel(p.type)}</b></span>
            <span><b class="font-semibold text-paper">{p.key_count}</b> key{p.key_count === 1 ? '' : 's'}</span>
            <span title="Upstream API key selection strategy">{selectionLabel(p.key_selection)}</span>
          </span>
          <span class="card-lift">
            <Lamp state={p.enabled ? 'on' : 'off'} label={p.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" />
          </span>
          <span class="card-lift flex items-center gap-2">
            <button
              class="btn-ghost"
              onclick={() => setEnabled(p, !p.enabled)}
              disabled={busyToggles.has(p.id)}
              aria-busy={busyToggles.has(p.id)}
            >
              <Busy busy={busyToggles.has(p.id)} text={p.enabled ? 'Disable' : 'Enable'} wide="Disable" />
            </button>
            {#if !p.builtin}
            <button class="btn-ghost" onclick={() => armed.confirm('p' + p.id, () => remove(p))}>
              <Fit text={armed.is('p' + p.id) ? 'Confirm?' : 'Delete'} wide="Confirm?" />
            </button>
            {/if}
          </span>
        </div>
      </li>
    {/each}
  </ol>
{/if}

{#if showCreate}
  <ProviderModal onclose={() => (showCreate = false)} />
{/if}
