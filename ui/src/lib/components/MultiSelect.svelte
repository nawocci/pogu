<script module lang="ts">
  import { nextDropdownId, matchOption } from './Dropdown.svelte';

  export interface MultiSelectOption {
    value: string;
    label: string;
    hint?: string;
    group?: string;
    disabled?: boolean;
    disabledReason?: string;
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';

  let {
    values = $bindable<string[]>([]),
    options,
    placeholder = 'Select…',
    label = '',
    disabled = false,
    searchPlaceholder = 'Search…',
    emptyText = 'No matches',
    class: klass = '',
  }: {
    values?: string[];
    options: MultiSelectOption[];
    placeholder?: string;
    label?: string;
    disabled?: boolean;
    searchPlaceholder?: string;
    emptyText?: string;
    class?: string;
  } = $props();

  const uid = nextDropdownId();
  let open = $state(false);
  let dropUp = $state(false);
  let query = $state('');
  let active = $state(0);
  let root = $state<HTMLDivElement | null>(null);
  let searchEl = $state<HTMLInputElement | null>(null);
  let listEl = $state<HTMLDivElement | null>(null);

  const selectedSet = $derived(new Set(values));
  const filtered = $derived(query.trim() === '' ? options : options.filter((o) => matchOption(o, query)));
  const cursor = $derived(filtered.length === 0 ? 0 : Math.min(Math.max(0, active), filtered.length - 1));

  function labelFor(v: string): string {
    return options.find((o) => o.value === v)?.label ?? v;
  }

  function openPicker() {
    if (disabled || open) return;
    query = '';
    open = true;
    active = 0;
    void tick().then(() => {
      searchEl?.focus();
      const r = root?.getBoundingClientRect();
      if (r) {
        const below = window.innerHeight - r.bottom;
        dropUp = below < 280 && r.top > below;
      }
    });
  }

  function closePicker(refocus = false) {
    if (!open) return;
    open = false;
    query = '';
    if (refocus) root?.querySelector('button')?.focus();
  }

  function toggle(v: string) {
    const opt = options.find((o) => o.value === v);
    if (opt?.disabled) return;
    if (selectedSet.has(v)) {
      values = values.filter((x) => x !== v);
    } else {
      values = [...values, v];
    }
  }

  function move(d: number) {
    if (filtered.length === 0) return;
    active = (active + d + filtered.length) % filtered.length;
    void tick().then(() => {
      listEl?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' });
    });
  }

  function onkeydown(e: KeyboardEvent) {
    if (disabled) return;
    if (!open) {
      if (e.key === 'ArrowDown' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        openPicker();
      }
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      closePicker(true);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      move(1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      move(-1);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const opt = filtered[cursor];
      if (opt) toggle(opt.value);
    }
  }

  function onfocusout(e: FocusEvent) {
    if (!open || !root) return;
    if (!(e.relatedTarget instanceof Node) || !root.contains(e.relatedTarget)) {
      closePicker();
    }
  }

  $effect(() => {
    if (!open) return;
    const onWindowClick = (e: MouseEvent) => {
      if (root && e.target instanceof Node && !root.contains(e.target)) closePicker();
    };
    window.addEventListener('click', onWindowClick);
    return () => window.removeEventListener('click', onWindowClick);
  });
</script>

<div bind:this={root} class="relative min-w-0 {klass || 'w-full'}" {onfocusout}>
  <div
    role="combobox"
    tabindex={disabled ? -1 : 0}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label={label || undefined}
    aria-controls="{uid}-list"
    aria-disabled={disabled || undefined}
    {onkeydown}
    onclick={() => (disabled ? undefined : open ? closePicker() : openPicker())}
    class="dd-trigger flex w-full items-start gap-2 rounded-sm border border-line bg-well px-2.5 py-1.5 text-left transition-colors hover:border-secondary {disabled ? 'cursor-wait opacity-45' : 'cursor-pointer'} max-compact:min-h-10"
  >
    <span class="min-w-0 flex-1">
      {#if values.length === 0}
        <span class="font-mono text-[13px] text-muted">{placeholder}</span>
      {:else}
        <span class="flex flex-wrap gap-1">
          {#each values as v (v)}
            <span class="inline-flex max-w-full items-center gap-1 rounded-[3px] border border-line bg-raised px-1.5 py-0.5 font-mono text-[11px] text-secondary">
              <span class="truncate">{labelFor(v)}</span>
              <button
                type="button"
                class="grid size-3.5 shrink-0 place-items-center rounded-[2px] text-muted hover:bg-hover hover:text-paper"
                aria-label={`Remove ${labelFor(v)}`}
                onclick={(e) => {
                  e.stopPropagation();
                  toggle(v);
                }}
              >×</button>
            </span>
          {/each}
        </span>
      {/if}
    </span>
    <span class="mt-0.5 grid shrink-0 place-items-center text-muted transition-transform duration-150 {open ? 'rotate-180' : ''}" aria-hidden="true">
      <Icon name="chevron" size={14} />
    </span>
  </div>

  {#if open}
    <div
      class="enter-blip absolute z-50 w-full overflow-hidden rounded-sm border border-line bg-raised-2 shadow-pop {dropUp
        ? 'bottom-full mb-1.5'
        : 'top-full mt-1.5'}"
    >
      <div class="border-b border-line-soft p-1.5">
        <div class="flex items-center gap-2 px-1.5 text-muted">
          <span class="grid shrink-0 place-items-center" aria-hidden="true"><Icon name="search" size={13} /></span>
          <input
            bind:this={searchEl}
            value={query}
            oninput={(e) => {
              query = e.currentTarget.value;
              active = 0;
            }}
            {onkeydown}
            placeholder={searchPlaceholder}
            role="combobox"
            aria-expanded={open}
            aria-controls="{uid}-list"
            aria-activedescendant={filtered.length > 0 ? `${uid}-opt-${cursor}` : undefined}
            aria-label="Filter options"
            autocomplete="off"
            spellcheck={false}
            class="min-w-0 flex-1 border-0 bg-transparent px-0 py-1 font-mono text-[12.5px] text-primary shadow-none placeholder:text-muted focus:border-0 focus:shadow-none max-compact:text-base"
          />
        </div>
      </div>
      <div
        bind:this={listEl}
        id="{uid}-list"
        role="listbox"
        aria-multiselectable="true"
        tabindex="-1"
        aria-label={label || placeholder}
        {onkeydown}
        class="max-h-[280px] overflow-y-auto p-1 outline-none"
      >
        {#if filtered.length === 0}
          <p class="m-0 px-3 py-5 text-center font-mono text-[12px] text-muted">{emptyText}</p>
        {:else}
          {#each filtered as opt, i (opt.value)}
            {@const isSel = selectedSet.has(opt.value)}
            <button
              type="button"
              role="option"
              id="{uid}-opt-{i}"
              aria-selected={isSel}
              aria-disabled={opt.disabled || undefined}
              data-active={i === cursor}
              title={opt.disabled ? opt.disabledReason ?? 'Unavailable' : [opt.label, opt.hint].filter(Boolean).join(' · ')}
              onmouseenter={() => (active = i)}
              onmousedown={(e) => e.preventDefault()}
              onclick={() => toggle(opt.value)}
              disabled={opt.disabled}
              class="flex w-full items-center gap-2 rounded-[2px] px-2.5 py-[7px] text-left font-mono text-[12.5px] transition-colors disabled:cursor-not-allowed disabled:opacity-40 max-compact:px-3 max-compact:py-2.5 {isSel
                ? 'bg-accent-dim text-paper'
                : i === cursor
                  ? 'bg-hover text-paper'
                  : 'text-secondary hover:bg-hover hover:text-paper'}"
            >
              <span class="grid size-3.5 shrink-0 place-items-center rounded-[2px] border border-line {isSel ? 'bg-accent text-ink' : ''}">
                {#if isSel}<Icon name="check" size={10} />{/if}
              </span>
              <span class="min-w-0 flex-1">
                <span class="flex min-w-0 items-center gap-2">
                  <span class="min-w-0 flex-1 truncate">{opt.label}</span>
                  {#if opt.group}
                    <span class="shrink-0 rounded-[2px] border border-line px-1 py-px text-[9.5px] tracking-[0.06em] text-muted uppercase">{opt.group}</span>
                  {/if}
                  {#if opt.hint}
                    <span class="max-w-[40%] shrink-0 truncate font-mono text-[10.5px] text-muted">{opt.hint}</span>
                  {/if}
                </span>
              </span>
            </button>
          {/each}
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .dd-trigger {
    outline: none;
  }
</style>
