<script lang="ts">
  import { api, groupInput, type Group, type GroupMember, type Model, type Provider } from '../api';
  import { store } from '../store.svelte';
  import { navigate } from '../router.svelte';
  import { toast } from '../toast.svelte';
  import { reveal } from '../motion.svelte';
  import { createArmed } from '../armed.svelte';
  import Lamp from '../components/Lamp.svelte';
  import Busy from '../components/Busy.svelte';
  import Fit from '../components/Fit.svelte';
  import Dropdown, { type DropdownOption } from '../components/Dropdown.svelte';
  import GroupModal from './GroupModal.svelte';

  let { groupId }: { groupId: number } = $props();

  let group = $state<Group | null>(null);
  let members = $state<GroupMember[]>([]);
  let candidates = $state<{ model: Model; provider: Provider | undefined }[]>([]);
  let loading = $state(true);
  let error = $state('');

  let selectedModelId = $state<number | ''>('');
  const candidateOptions = $derived(
    candidates.map(
      (c): DropdownOption => ({
        value: String(c.model.id),
        label: c.model.public_id,
        hint: c.provider ? c.provider.prefix : undefined,
      }),
    ),
  );
  let showEdit = $state(false);
  let busyMember = $state<number | null>(null);
  let busyOrder = $state(false);
  let busyToggle = $state(false);
  const armed = createArmed();

  async function loadDetail() {
    loading = true;
    error = '';
    try {
      const [g, gm, allModels, allProviders] = await Promise.all([
        api.getGroup(groupId),
        api.groupMembers(groupId),
        api.models(),
        api.providers(),
      ]);
      group = g;
      members = gm ?? [];
      const memberModelIds = new Set(members.map((m) => m.model_id));
      candidates = (allModels ?? [])
        .filter((m) => m.enabled && !memberModelIds.has(m.id))
        .map((m) => ({ model: m, provider: (allProviders ?? []).find((p) => p.id === m.provider_id) }));
      if (selectedModelId !== '' && !candidates.some((c) => c.model.id === selectedModelId)) {
        selectedModelId = '';
      }
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

  async function addMember() {
    if (selectedModelId === '') return;
    const modelId = selectedModelId;
    try {
      await api.addGroupMember(groupId, modelId);
      selectedModelId = '';
      await loadDetail();
      await store.refresh();
      toast('success', 'Target added');
    } catch (e) {
      fail(e);
    }
  }

  async function toggleMember(member: GroupMember) {
    if (busyMember) return;
    busyMember = member.id;
    try {
      await api.updateGroupMember(groupId, member.id, !member.enabled);
      await loadDetail();
    } catch (e) {
      fail(e);
    } finally {
      busyMember = null;
    }
  }

  async function removeMember(member: GroupMember) {
    try {
      await api.deleteGroupMember(groupId, member.id);
      await loadDetail();
      await store.refresh();
      toast('success', `Target "${member.public_id}" removed`);
    } catch (e) {
      fail(e);
    }
  }

  async function reorder(memberIds: number[]) {
    busyOrder = true;
    try {
      members = await api.reorderGroupMembers(groupId, memberIds);
    } catch (e) {
      fail(e);
    } finally {
      busyOrder = false;
    }
  }

  async function move(index: number, delta: -1 | 1) {
    const ids = members.map((m) => m.id);
    const j = index + delta;
    if (j < 0 || j >= ids.length) return;
    [ids[index], ids[j]] = [ids[j], ids[index]];
    await reorder(ids);
  }

  async function setSelection(mode: 'first' | 'round_robin') {
    if (!group || group.selection === mode) return;
    try {
      group = await api.setGroupSelection(groupId, mode);
      toast('success', mode === 'round_robin' ? 'Strategy set to round robin' : 'Strategy set to first');
    } catch (e) {
      fail(e);
    }
  }

  async function setEnabled(enabled: boolean) {
    const current = group;
    if (!current || busyToggle) return;
    busyToggle = true;
    try {
      group = await store.mutate(() => api.updateGroup(groupId, groupInput(current, { enabled })));
    } catch (e) {
      fail(e);
    } finally {
      busyToggle = false;
    }
  }

  async function deleteGroup() {
    if (!group) return;
    try {
      const name = group.name;
      await api.deleteGroup(group.id);
      await store.refresh();
      toast('success', `Group "${name}" deleted`);
      navigate('/groups', { replace: true });
    } catch (e) {
      fail(e);
    }
  }
</script>

{#if loading && !group}
  <p class="flex items-center gap-3 py-10 text-sm text-tertiary"><Lamp state="live" /> Loading…</p>
{:else if error && !group}
  <p class="error-line">{error}</p>
{:else if !group}
  <div class="empty">
    <h2 class="mb-2">Group not found</h2>
    <p class="mt-2 mb-[18px] text-tertiary">It may have been deleted.</p>
    <a class="btn" href="/groups">Back to groups</a>
  </div>
{:else}
  <a
    use:reveal={{ kind: 'blip' }}
    class="back-link"
    href="/groups"
  >← Groups</a>

  <header use:reveal={{ kind: 'rise', i: 1 }} class="mt-4 mb-8 flex flex-wrap items-start justify-between gap-5">
    <div>
      <h1 class="leading-none">{group.name}</h1>
      <p class="mt-2.5 flex flex-wrap items-center gap-2.5 text-[13px] text-tertiary">
        <span>Logical model group</span>
        <span class="text-muted">·</span>
        <span>clients use <code class="font-mono text-[11.5px] text-primary">"model": "{group.name}"</code></span>
      </p>
    </div>
    <div use:reveal={{ kind: 'pop', i: 2 }} class="flex flex-wrap items-center gap-2">
      <Lamp state={group.enabled ? 'on' : 'off'} label={group.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" />
      <button class="btn btn-sm" onclick={() => group && setEnabled(!group.enabled)} disabled={busyToggle} aria-busy={busyToggle}>
        <Busy busy={busyToggle} text={group.enabled ? 'Disable' : 'Enable'} wide="Disable" />
      </button>
      <button class="btn btn-sm" onclick={() => (showEdit = true)}>Edit</button>
      <button
        class="btn btn-sm btn-danger"
        class:btn-armed={armed.is('group')}
        onclick={() => armed.confirm('group', deleteGroup)}
      >
        <Fit text={armed.is('group') ? 'Confirm delete' : 'Delete'} wide="Confirm delete" />
      </button>
    </div>
  </header>

  <section use:reveal={{ kind: 'rise', i: 2 }} aria-labelledby="strategy-h">
    <div class="sec-head">
      <h2 id="strategy-h">Selection strategy</h2>
    </div>
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-sm border border-line bg-raised px-3.5 py-2.5">
      <div class="flex gap-0.5 rounded-sm border border-line bg-well p-0.5" role="group" aria-label="Selection strategy">
        <button
          aria-pressed={group.selection === 'first'}
          class="rounded-[2px] px-3 py-1 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {group.selection === 'first'
            ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
            : 'text-tertiary hover:text-primary'}"
          onclick={() => setSelection('first')}
        >First</button>
        <button
          aria-pressed={group.selection === 'round_robin'}
          class="rounded-[2px] px-3 py-1 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {group.selection === 'round_robin'
            ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
            : 'text-tertiary hover:text-primary'}"
          onclick={() => setSelection('round_robin')}
        >Round robin</button>
      </div>
      <p class="m-0 text-[12.5px] text-tertiary">
        {group.selection === 'round_robin'
          ? 'Requests rotate the starting target; failover follows the configured order.'
          : 'Requests start at the first available target and fail over down the list.'}
      </p>
    </div>
  </section>

  <section use:reveal={{ kind: 'rise', i: 3 }} aria-labelledby="targets-h" class="mt-[42px]">
    <div class="sec-head">
      <h2 id="targets-h">Targets <span class="ml-[5px] font-mono text-[13px] text-accent-ink [vertical-align:3px]">{members.length}</span></h2>
    </div>
    <p class="mb-4 max-w-[80ch] text-[13px] text-tertiary">
      Concrete models this group resolves to, tried in order. On an upstream 401, 403, or 429 the request moves
      to the next target before any response is sent. A single target makes this group a stable alias.
    </p>

    <form
      class="mb-4 flex flex-wrap items-center gap-2.5"
      onsubmit={(e) => {
        e.preventDefault();
        void addMember();
      }}
    >
      <Dropdown
        class="w-full max-w-[420px]"
        label="Model to add"
        placeholder="Select a model to add…"
        searchable
        searchPlaceholder="Search models or prefixes…"
        emptyText={candidates.length === 0 ? 'No available models' : 'No models match'}
        disabled={candidates.length === 0}
        options={candidateOptions}
        value={selectedModelId === '' ? '' : String(selectedModelId)}
        onchange={(v) => (selectedModelId = v === '' ? '' : Number(v))}
      />
      <button type="submit" class="btn btn-primary" disabled={selectedModelId === ''}>Add target</button>
    </form>

    {#if members.length === 0}
      <p class="text-[13px] text-tertiary">No targets yet. Requests to this group will fail until at least one target is available.</p>
    {:else}
      <table class="table-data">
        <thead>
          <tr><th>Order</th><th>Target</th><th>Provider</th><th>Status</th><th></th></tr>
        </thead>
        <tbody>
          {#each members as m, i (m.id)}
            {@const armedNow = armed.is('m' + m.id)}
            <tr>
              <td class="whitespace-nowrap">
                <span class="font-mono text-[11.5px] text-tertiary">#{i + 1}</span>
                <button
                  class="btn-ghost btn-ghost-xs ml-1.5"
                  disabled={i === 0 || busyOrder}
                  onclick={() => move(i, -1)}
                  aria-label="Move up"
                >↑</button>
                <button
                  class="btn-ghost btn-ghost-xs"
                  disabled={i === members.length - 1 || busyOrder}
                  onclick={() => move(i, 1)}
                  aria-label="Move down"
                >↓</button>
              </td>
              <td><code class="font-mono text-[12.5px] font-semibold text-paper">{m.public_id}</code></td>
              <td class="font-mono text-[12px] text-secondary">{m.provider}</td>
              <td><Lamp state={m.enabled ? 'on' : 'off'} label={m.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" /></td>
              <td class="text-right whitespace-nowrap">
                <button class="linkish" onclick={() => toggleMember(m)} disabled={busyMember === m.id} aria-busy={busyMember === m.id}>
                  <Busy busy={busyMember === m.id} text={m.enabled ? 'Disable' : 'Enable'} wide="Disable" />
                </button>
                <button class="linkish linkish-del" onclick={() => armed.confirm('m' + m.id, () => removeMember(m))}>
                  <Fit text={armedNow ? 'Confirm?' : 'Remove'} wide="Confirm?" />
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>
{/if}

{#if group && showEdit}
  <GroupModal
    {group}
    onclose={() => (showEdit = false)}
    onsaved={() => {
      void loadDetail();
      void store.refresh();
    }}
  />
{/if}
