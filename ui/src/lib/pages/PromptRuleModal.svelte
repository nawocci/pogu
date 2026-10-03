<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import Busy from '../components/Busy.svelte';
  import MultiSelect, { type MultiSelectOption } from '../components/MultiSelect.svelte';
  import { api, type PromptRule, type PromptRuleTarget } from '../api';
  import { toast } from '../toast.svelte';

  let {
    rule,
    options,
    onclose,
    onsaved,
  }: {
    rule: PromptRule | null;
    options: MultiSelectOption[];
    onclose: () => void;
    onsaved: () => void;
  } = $props();

  const MAX_PROMPT = 16384;

  /* svelte-ignore state_referenced_locally */
  let name = $state(rule?.name ?? '');
  /* svelte-ignore state_referenced_locally */
  let prompt = $state(rule?.prompt ?? '');
  /* svelte-ignore state_referenced_locally */
  let inherit = $state(rule?.inherit ?? true);
  /* svelte-ignore state_referenced_locally */
  let enabled = $state(rule?.enabled ?? true);
  /* svelte-ignore state_referenced_locally */
  let selected = $state<string[]>(
    (rule?.targets ?? []).map((t: PromptRuleTarget) => `${t.target_type}:${t.target_id}`),
  );
  let busy = $state(false);
  let error = $state('');

  function parseTargets(values: string[]): PromptRuleTarget[] {
    return values.map((v) => {
      const [target_type, id] = v.split(':');
      const opt = options.find((o) => o.value === v);
      return {
        target_type: target_type as 'group' | 'model',
        target_id: Number(id),
        label: opt?.label ?? v,
      };
    });
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (!name.trim()) {
      error = 'Rule name is required.';
      return;
    }
    if (prompt.length > MAX_PROMPT) {
      error = `System prompt must be at most ${MAX_PROMPT} characters.`;
      return;
    }
    if (selected.length === 0) {
      error = 'Select at least one target.';
      return;
    }
    busy = true;
    try {
      const input = {
        name: name.trim(),
        prompt,
        inherit,
        enabled,
        targets: parseTargets(selected),
      };
      if (rule) {
        await api.updatePromptRule(rule.id, input);
        toast('success', `Rule "${input.name}" updated`);
      } else {
        await api.createPromptRule(input);
        toast('success', `Rule "${input.name}" created`);
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

<Modal title={rule ? 'Edit prompt rule' : 'New prompt rule'} onclose={onclose} wide={true}>
  <form novalidate onsubmit={save} class="flex flex-col gap-4">
    <label class="field">
      <span>Rule name</span>
      <input type="text" bind:value={name} placeholder="Tool-use correction, Coding agents, …" />
      <p class="hint">Shown in the injected marker and recorded in telemetry (name only).</p>
    </label>

    <label class="field">
      <span>Applies to</span>
      <MultiSelect
        bind:values={selected}
        options={options}
        placeholder="Select models or groups…"
        searchPlaceholder="Search models or groups…"
        emptyText="No targets available"
        label="Rule targets"
      />
      <p class="hint">
        A group target fires only when clients call the group by name. A model target fires whenever that model
        serves a request — directly or as a group's resolved target.
      </p>
    </label>

    <label class="field">
      <span>System prompt</span>
      <textarea
        bind:value={prompt}
        placeholder="e.g. Use the provided tools instead of shell commands."
        class="min-h-[160px] font-mono"
      ></textarea>
      <span class="mt-1 flex items-center justify-end font-mono text-[11px] text-tertiary">{prompt.length} / {MAX_PROMPT}</span>
    </label>

    <div class="flex flex-col gap-2.5">
      <label class="flex items-center gap-2 text-[13px] text-secondary">
        <input type="checkbox" bind:checked={inherit} class="size-4" />
        Also apply global prompt
      </label>
      <label class="flex items-center gap-2 text-[13px] text-secondary">
        <input type="checkbox" bind:checked={enabled} class="size-4" />
        Enabled
      </label>
    </div>

    {#if error}<p class="error-line enter-blip">{error}</p>{/if}
    <div class="modal-actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary" disabled={busy} aria-busy={busy}>
        <Busy busy={busy} text={rule ? 'Save changes' : 'Create rule'} wide="Save changes" />
      </button>
    </div>
  </form>
</Modal>
