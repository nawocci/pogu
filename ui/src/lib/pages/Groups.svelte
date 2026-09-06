<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { api, groupInput, type Group } from '../api';
  import { store } from '../store.svelte';
  import { toast } from '../toast.svelte';
  import { reveal } from '../motion.svelte';
  import { createArmed } from '../armed.svelte';
  import Lamp from '../components/Lamp.svelte';
  import Busy from '../components/Busy.svelte';
  import GroupModal from './GroupModal.svelte';
  import Fit from '../components/Fit.svelte';

  let showCreate = $state(false);
  const armed = createArmed();

  const groups = $derived(store.groups);

  async function refresh() {
    await store.refresh();
  }

  async function remove(group: Group) {
    try {
      await store.mutate(() => api.deleteGroup(group.id));
      toast('success', `Group "${group.name}" deleted`);
    } catch (e) {
      toast('error', e instanceof Error ? e.message : String(e));
    }
  }

  const busyToggles = new SvelteSet<number>();

  async function setEnabled(g: Group, enabled: boolean) {
    if (busyToggles.has(g.id)) return;
    busyToggles.add(g.id);
    try {
      await store.mutate(() => api.updateGroup(g.id, groupInput(g, { enabled })));
    } catch (e) {
      toast('error', e instanceof Error ? e.message : String(e));
    } finally {
      busyToggles.delete(g.id);
    }
  }

  const sorted = $derived([...groups].sort((a, b) => a.name.localeCompare(b.name)));
</script>

<header use:reveal={{ kind: 'rise', i: 0 }} class="mb-8 flex flex-wrap items-end justify-between gap-5">
  <div>
    <p class="mb-4 font-mono text-[11px] tracking-[0.1em] text-accent-ink uppercase">Management</p>
    <h1 class="leading-none">Groups</h1>
    <p class="mt-2.5 text-sm text-tertiary">Logical model names that resolve to concrete <code class="font-mono text-xs">prefix/model</code> targets.</p>
  </div>
  <button use:reveal={{ kind: 'pop', i: 1 }} class="btn btn-primary" onclick={() => (showCreate = true)}>Add group</button>
</header>

{#if store.error}
  <p use:reveal={{ kind: 'blip' }} class="error-line">{store.error}</p>
{/if}

{#if !store.loaded}
  <div class="flex items-center gap-3 py-10 text-sm text-tertiary"><Lamp state="live" /> Loading groups…</div>
{:else if sorted.length === 0}
  <div use:reveal={{ kind: 'blip' }} class="empty">
    <h2 class="mb-2">No groups yet</h2>
    <p class="mt-2 mb-4 max-w-[52ch] text-tertiary">
      A one-member group is a stable alias; multiple members form a fallback pool tried in order.
    </p>
    <button class="btn btn-primary" onclick={() => (showCreate = true)}>Add group</button>
  </div>
{:else}
  <ol class="m-0 flex list-none flex-col gap-2.5 p-0" aria-label="Configured groups">
    {#each sorted as g (g.id)}
      <li use:reveal={{ kind: 'blip', i: 1 }}>
        <div class="card-row">
          <a class="card-hit" href={'/groups/' + g.id} aria-label={g.name}></a>
          <span class="flex min-w-0 flex-col gap-1">
            <span class="inline-flex min-w-0 items-center gap-2">
              <span class="overflow-hidden font-heading text-xl leading-[1.2] font-semibold tracking-[-0.01em] text-ellipsis whitespace-nowrap text-paper">{g.name}</span>
            </span>
            <code class="truncate font-mono text-[11.5px] leading-[1.4] text-tertiary">"model": "{g.name}"</code>
          </span>
          <span class="flex gap-[18px] font-mono text-[11.5px] whitespace-nowrap text-tertiary max-wide:hidden">
            <span><b class="font-semibold text-paper">{g.member_count}</b> target{g.member_count === 1 ? '' : 's'}</span>
            <span>{g.selection === 'round_robin' ? 'round robin' : 'first'}</span>
          </span>
          <span class="card-lift">
            <Lamp state={g.enabled ? 'on' : 'off'} label={g.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" />
          </span>
          <span class="card-lift flex items-center gap-2 card-actions">
            <button
              class="btn-ghost"
              onclick={() => setEnabled(g, !g.enabled)}
              disabled={busyToggles.has(g.id)}
              aria-busy={busyToggles.has(g.id)}
            >
              <Busy busy={busyToggles.has(g.id)} text={g.enabled ? 'Disable' : 'Enable'} wide="Disable" />
            </button>
            <button
              class="btn-ghost"
              onclick={() => armed.confirm('g' + g.id, () => remove(g))}
            >
              <Fit text={armed.is('g' + g.id) ? 'Confirm?' : 'Delete'} wide="Confirm?" />
            </button>
          </span>
        </div>
      </li>
    {/each}
  </ol>
{/if}

{#if showCreate}
  <GroupModal
    onclose={() => (showCreate = false)}
    onsaved={() => void refresh()}
  />
{/if}
