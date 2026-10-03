<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type PromptRule, type PromptRuleTarget } from '../api';
  import { store } from '../store.svelte';
  import { toast } from '../toast.svelte';
  import { reveal } from '../motion.svelte';
  import { createArmed } from '../armed.svelte';
  import type { MultiSelectOption } from '../components/MultiSelect.svelte';
  import Busy from '../components/Busy.svelte';
  import Fit from '../components/Fit.svelte';
  import Lamp from '../components/Lamp.svelte';
  import PromptRuleModal from './PromptRuleModal.svelte';

  const MAX_PROMPT = 16384;

  // Global base layer (settings-backed, pinned, not part of the reorderable list).
  let globalPrompt = $state('');
  let globalEnabled = $state(false);
  let globalLoaded = $state(false);
  let busyGlobal = $state(false);
  let globalError = $state('');

  // Ordered rule list.
  let rules = $state<PromptRule[]>([]);
  let rulesLoaded = $state(false);
  let rulesError = $state('');
  let busyOrder = $state(false);
  const busyToggles = $state<Record<number, boolean>>({});
  const armed = createArmed();

  let showCreate = $state(false);
  let editing = $state<PromptRule | null>(null);

  const targetOptions = $derived.by<MultiSelectOption[]>(() => {
    const claimed = new Map<string, string>();
    for (const r of rules) {
      if (editing && r.id === editing.id) continue;
      for (const t of r.targets) claimed.set(`${t.target_type}:${t.target_id}`, r.name);
    }
    const opts: MultiSelectOption[] = [];
    for (const g of store.groups) {
      const key = `group:${g.id}`;
      const owner = claimed.get(key);
      opts.push({
        value: key,
        label: g.name,
        group: 'group',
        disabled: Boolean(owner),
        disabledReason: owner ? `Used by "${owner}"` : undefined,
      });
    }
    for (const m of store.models) {
      const key = `model:${m.id}`;
      const owner = claimed.get(key);
      opts.push({
        value: key,
        label: m.public_id,
        hint: m.provider,
        group: 'model',
        disabled: Boolean(owner),
        disabledReason: owner ? `Used by "${owner}"` : undefined,
      });
    }
    return opts;
  });

  onMount(() => {
    void loadGlobal();
    void loadRules();
  });

  async function loadGlobal() {
    try {
      const p = await api.globalPrompt();
      globalPrompt = p.system_prompt;
      globalEnabled = p.enabled;
      globalLoaded = true;
    } catch (e) {
      globalError = e instanceof Error ? e.message : String(e);
    }
  }

  async function loadRules() {
    try {
      rules = await api.promptRules();
      rulesLoaded = true;
    } catch (e) {
      rulesError = e instanceof Error ? e.message : String(e);
    }
  }

  async function saveGlobal(e: SubmitEvent) {
    e.preventDefault();
    globalError = '';
    if (globalPrompt.length > MAX_PROMPT) {
      globalError = `System prompt must be at most ${MAX_PROMPT} characters.`;
      return;
    }
    busyGlobal = true;
    try {
      const p = await api.updateGlobalPrompt({ system_prompt: globalPrompt, enabled: globalEnabled });
      globalPrompt = p.system_prompt;
      globalEnabled = p.enabled;
      toast('success', 'Global system prompt saved');
    } catch (err) {
      globalError = err instanceof Error ? err.message : String(err);
    } finally {
      busyGlobal = false;
    }
  }

  function fail(e: unknown) {
    toast('error', e instanceof Error ? e.message : String(e));
  }

  async function toggleRule(rule: PromptRule, enabled: boolean) {
    if (busyToggles[rule.id]) return;
    busyToggles[rule.id] = true;
    try {
      const updated = await api.updatePromptRule(rule.id, {
        name: rule.name,
        prompt: rule.prompt,
        inherit: rule.inherit,
        enabled,
        targets: rule.targets,
      });
      rules = rules.map((r) => (r.id === updated.id ? updated : r));
    } catch (e) {
      fail(e);
    } finally {
      busyToggles[rule.id] = false;
    }
  }

  async function removeRule(rule: PromptRule) {
    try {
      await api.deletePromptRule(rule.id);
      rules = rules.filter((r) => r.id !== rule.id);
      toast('success', `Rule "${rule.name}" deleted`);
    } catch (e) {
      fail(e);
    }
  }

  async function move(index: number, delta: -1 | 1) {
    const j = index + delta;
    if (j < 0 || j >= rules.length) return;
    const next = [...rules];
    [next[index], next[j]] = [next[j], next[index]];
    busyOrder = true;
    const prev = rules;
    rules = next;
    try {
      rules = await api.reorderPromptRules(next.map((r) => r.id));
    } catch (e) {
      rules = prev;
      fail(e);
    } finally {
      busyOrder = false;
    }
  }

  function targetChips(targets: PromptRuleTarget[]): string[] {
    return targets.map((t) => t.label);
  }
</script>

<header use:reveal={{ kind: 'rise', i: 0 }} class="mb-8 flex flex-wrap items-end justify-between gap-5">
  <div>
    <p class="mb-4 font-mono text-[11px] tracking-[0.1em] text-accent-ink uppercase">Management</p>
    <h1 class="leading-none">System prompts</h1>
    <p class="mt-2.5 max-w-[70ch] text-sm text-tertiary">
      A global base applied to every request, plus ordered rules that add a prompt when their targets are called.
      Rules stack in list order on top of the base.
    </p>
  </div>
  <button use:reveal={{ kind: 'pop', i: 1 }} class="btn btn-primary" onclick={() => (showCreate = true)}>
    New rule
  </button>
</header>

<div class="flex flex-col gap-y-[42px]">
  <section use:reveal={{ kind: 'rise', i: 1 }} aria-labelledby="base-h">
    <div class="sec-head">
      <h2 id="base-h">Base layer</h2>
      <span class="rounded-[3px] border border-line px-1.5 py-0.5 font-mono text-[10px] tracking-[0.07em] text-tertiary uppercase">Global</span>
    </div>
    <p class="mb-4 max-w-[80ch] text-[13px] text-tertiary">
      Applies to every request first, unless a matching rule turns inheritance off. Always injected before rule
      prompts, and never reordered.
    </p>
    {#if !globalLoaded && !globalError}
      <p class="flex items-center gap-3 py-6 text-sm text-tertiary">Loading…</p>
    {:else}
      <form novalidate onsubmit={saveGlobal} class="flex flex-col">
        <label class="field">
          <span>Preferences applied to every request</span>
          <textarea
            bind:value={globalPrompt}
            placeholder="e.g. Always answer concisely."
            class="min-h-[200px] font-mono"
          ></textarea>
          <p class="hint">
            Wrapped in a <code class="font-mono text-xs">Pogu router preferences</code> marker so it stays
            identifiable. Telemetry never stores prompt text.
          </p>
        </label>
        <div class="mb-4 flex items-center justify-between gap-4">
          <label class="flex items-center gap-2 text-[13px] text-secondary">
            <input type="checkbox" bind:checked={globalEnabled} class="size-4" />
            Enabled
          </label>
          <span class="font-mono text-[11px] text-tertiary">{globalPrompt.length} / {MAX_PROMPT}</span>
        </div>
        {#if globalError}<p class="error-line enter-blip">{globalError}</p>{/if}
        <div class="modal-actions">
          <button type="submit" class="btn btn-primary" disabled={busyGlobal} aria-busy={busyGlobal}>
            <Busy busy={busyGlobal} text="Save base prompt" wide="Save base prompt" />
          </button>
        </div>
      </form>
    {/if}
  </section>

  <section use:reveal={{ kind: 'rise', i: 2 }} aria-labelledby="rules-h">
    <div class="sec-head">
      <h2 id="rules-h">
        Rules <span class="ml-[5px] font-mono text-[13px] text-accent-ink [vertical-align:3px]">{rules.length}</span>
      </h2>
    </div>
    <p class="mb-4 max-w-[80ch] text-[13px] text-tertiary">
      Applied in order after the base. Each rule may target groups, models, or both; a target can only be
      claimed by one rule.
    </p>

    {#if rulesError}
      <p class="error-line">{rulesError}</p>
    {:else if !rulesLoaded}
      <p class="flex items-center gap-3 py-6 text-sm text-tertiary">Loading…</p>
    {:else if rules.length === 0}
      <div class="empty">
        <h3 class="mb-2">No rules yet</h3>
        <p class="mt-2 mb-4 max-w-[52ch] text-tertiary">
          Add a rule to attach a system prompt to specific models or groups — for example, correcting a model
          that ignores available tools.
        </p>
        <button class="btn btn-primary" onclick={() => (showCreate = true)}>New rule</button>
      </div>
    {:else}
      <ol class="m-0 flex list-none flex-col gap-2.5 p-0" aria-label="Prompt rules">
        {#each rules as rule, i (rule.id)}
          {@const armedNow = armed.is('r' + rule.id)}
          <li use:reveal={{ kind: 'blip' }}>
            <div class="flex flex-col gap-3 rounded-sm border border-line bg-raised px-3.5 py-3">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div class="flex min-w-0 flex-col gap-1.5">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-mono text-[11px] text-tertiary">#{i + 1}</span>
                    <span class="font-heading text-[15px] font-semibold tracking-[-0.01em] text-paper">{rule.name}</span>
                    <Lamp state={rule.enabled ? 'on' : 'off'} label={rule.enabled ? 'Enabled' : 'Disabled'} wide="Disabled" />
                    {#if !rule.inherit}
                      <span class="rounded-[3px] border border-line px-1.5 py-0.5 font-mono text-[10px] tracking-[0.06em] text-tertiary uppercase">No global</span>
                    {/if}
                  </div>
                  <div class="flex flex-wrap gap-1">
                    {#each targetChips(rule.targets) as chip (chip)}
                      <span class="rounded-[3px] border border-line bg-well px-1.5 py-0.5 font-mono text-[11px] text-secondary">{chip}</span>
                    {/each}
                  </div>
                </div>
                <div class="flex shrink-0 items-center gap-1">
                  <button
                    class="btn-ghost btn-ghost-xs"
                    disabled={i === 0 || busyOrder}
                    onclick={() => move(i, -1)}
                    aria-label="Move up"
                  >↑</button>
                  <button
                    class="btn-ghost btn-ghost-xs"
                    disabled={i === rules.length - 1 || busyOrder}
                    onclick={() => move(i, 1)}
                    aria-label="Move down"
                  >↓</button>
                  <button class="btn-ghost btn-ghost-xs" onclick={() => (editing = rule)}>Edit</button>
                  <button
                    class="btn-ghost btn-ghost-xs"
                    onclick={() => toggleRule(rule, !rule.enabled)}
                    disabled={busyToggles[rule.id]}
                    aria-busy={busyToggles[rule.id]}
                  >
                    <Busy busy={busyToggles[rule.id]} text={rule.enabled ? 'Disable' : 'Enable'} wide="Disable" />
                  </button>
                  <button
                    class="btn-ghost btn-ghost-xs linkish-del"
                    onclick={() => armed.confirm('r' + rule.id, () => removeRule(rule))}
                  >
                    <Fit text={armedNow ? 'Confirm?' : 'Delete'} wide="Confirm?" />
                  </button>
                </div>
              </div>
              {#if rule.prompt.trim()}
                <p class="m-0 line-clamp-3 border-t border-line-soft pt-2.5 font-mono text-[12px] leading-[1.5] text-tertiary whitespace-pre-wrap">{rule.prompt}</p>
              {/if}
            </div>
          </li>
        {/each}
      </ol>
    {/if}
  </section>
</div>

{#if showCreate}
  <PromptRuleModal
    rule={null}
    options={targetOptions}
    onclose={() => (showCreate = false)}
    onsaved={() => void loadRules()}
  />
{/if}

{#if editing}
  <PromptRuleModal
    rule={editing}
    options={targetOptions}
    onclose={() => (editing = null)}
    onsaved={() => void loadRules()}
  />
{/if}
