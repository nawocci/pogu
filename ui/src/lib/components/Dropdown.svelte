<script module lang="ts">
  export interface DropdownOption {
    value: string;
    label: string;
    hint?: string;
  }

  let uidCounter = 0;
  export function nextDropdownId(): string {
    uidCounter += 1;
    return `dd-${uidCounter}`;
  }

  export function matchOption(o: DropdownOption, query: string): boolean {
    const hay = `${o.label} ${o.hint ?? ''} ${o.value}`.toLowerCase();
    return query
      .toLowerCase()
      .split(/\s+/)
      .filter(Boolean)
      .every((t) => hay.includes(t));
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';
  import Icon from './Icon.svelte';

  let {
    value = $bindable(''),
    options,
    placeholder = 'Select…',
    label = '',
    disabled = false,
    searchable = false,
    searchPlaceholder = 'Search…',
    emptyText = 'No matches',
    size = 'md',
    class: klass = '',
    onchange,
  }: {
    value?: string;
    options: DropdownOption[];
    placeholder?: string;
    label?: string;
    disabled?: boolean;
    searchable?: boolean;
    searchPlaceholder?: string;
    emptyText?: string;
    size?: 'md' | 'sm';
    class?: string;
    onchange?: (value: string) => void;
  } = $props();

  const uid = nextDropdownId();
  let open = $state(false);
  let dropUp = $state(false);
  let query = $state('');
  let active = $state(0);
  let root = $state<HTMLDivElement | null>(null);
  let triggerEl = $state<HTMLButtonElement | null>(null);
  let searchEl = $state<HTMLInputElement | null>(null);
  let listEl = $state<HTMLDivElement | null>(null);

  const selected = $derived(options.find((o) => o.value === value));
  const isPlaceholder = $derived(!selected && value === '');
  const shown = $derived(selected?.label ?? (value !== '' ? value : placeholder));
  const filtered = $derived(query.trim() === '' ? options : options.filter((o) => matchOption(o, query)));
  const cursor = $derived(filtered.length === 0 ? 0 : Math.min(Math.max(0, active), filtered.length - 1));

  const sm = $derived(size === 'sm');

  function openDropdown() {
    if (disabled || open) return;
    query = '';
    open = true;
    const i = options.findIndex((o) => o.value === value);
    active = i >= 0 ? i : 0;
    void tick().then(() => {
      if (searchable) {
        searchEl?.focus();
      } else {
        listEl?.focus();
      }
      const r = root?.getBoundingClientRect();
      if (r) {
        const below = window.innerHeight - r.bottom;
        dropUp = below < 280 && r.top > below;
      }
    });
  }

  function closeDropdown(refocus = false) {
    if (!open) return;
    open = false;
    query = '';
    if (refocus) triggerEl?.focus();
  }

  function select(v: string) {
    value = v;
    onchange?.(v);
    closeDropdown(true);
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
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        openDropdown();
        if (e.key === 'ArrowUp') active = Math.max(0, options.length - 1);
      }
      return;
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      closeDropdown(true);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      move(1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      move(-1);
    } else if (e.key === 'Enter' || (e.key === ' ' && !(e.target instanceof HTMLInputElement))) {
      e.preventDefault();
      const opt = filtered[cursor];
      if (opt) select(opt.value);
    }
  }

  function onfocusout(e: FocusEvent) {
    if (!open || !root) return;
    if (!(e.relatedTarget instanceof Node) || !root.contains(e.relatedTarget)) {
      closeDropdown();
    }
  }

  $effect(() => {
    if (!open) return;
    const onWindowClick = (e: MouseEvent) => {
      if (root && e.target instanceof Node && !root.contains(e.target)) closeDropdown();
    };
    window.addEventListener('click', onWindowClick);
    return () => window.removeEventListener('click', onWindowClick);
  });
</script>

<div bind:this={root} class="relative min-w-0 {klass || 'w-full'}" {onfocusout}>
  <button
    bind:this={triggerEl}
    type="button"
    {disabled}
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label={label || undefined}
    aria-controls="{uid}-list"
    {onkeydown}
    onclick={() => (open ? closeDropdown() : openDropdown())}
    class="dd-trigger flex w-full items-center gap-2 rounded-sm border border-line bg-well transition-colors hover:border-secondary disabled:cursor-wait disabled:opacity-45 {sm
      ? 'px-2 py-1 font-mono text-[11px]'
      : 'px-3 py-2 font-mono text-[13px]'} {open ? 'dd-open' : ''}"
  >
    <span class="min-w-0 flex-1 truncate text-left {isPlaceholder ? 'text-muted' : 'text-primary'}">{shown}</span>
    <span class="grid shrink-0 place-items-center text-muted transition-transform duration-150 {open ? 'rotate-180' : ''}" aria-hidden="true">
      <Icon name="chevron" size={sm ? 12 : 14} />
    </span>
  </button>

  {#if open}
    <div
      class="enter-blip absolute z-50 w-full overflow-hidden rounded-sm border border-line bg-raised-2 shadow-pop {dropUp
        ? 'bottom-full mb-1.5'
        : 'top-full mt-1.5'}"
    >
      {#if searchable}
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
              class="min-w-0 flex-1 border-0 bg-transparent px-0 py-1 font-mono text-[12.5px] text-primary shadow-none placeholder:text-muted focus:border-0 focus:shadow-none"
            />
          </div>
        </div>
      {/if}
      <div
        bind:this={listEl}
        id="{uid}-list"
        role="listbox"
        tabindex="-1"
        aria-label={label || placeholder}
        aria-activedescendant={searchable ? undefined : filtered.length > 0 ? `${uid}-opt-${cursor}` : undefined}
        {onkeydown}
        class="max-h-[248px] overflow-y-auto p-1 outline-none"
      >
        {#if filtered.length === 0}
          <p class="m-0 px-3 py-5 text-center font-mono text-[12px] text-muted">{emptyText}</p>
        {:else}
          {#each filtered as opt, i (opt.value)}
            {@const isSel = opt.value === value}
            <button
              type="button"
              role="option"
              id="{uid}-opt-{i}"
              aria-selected={isSel}
              data-active={i === cursor}
              title={opt.hint ? `${opt.label} · ${opt.hint}` : opt.label}
              onmouseenter={() => (active = i)}
              onmousedown={(e) => e.preventDefault()}
              onclick={() => select(opt.value)}
              class="flex w-full items-center gap-2 rounded-[2px] px-2.5 py-[7px] text-left font-mono text-[12.5px] transition-colors {isSel
                ? 'bg-accent-dim text-paper'
                : i === cursor
                  ? 'bg-hover text-paper'
                  : 'text-secondary hover:bg-hover hover:text-paper'}"
            >
              <span class="min-w-0 flex-1 truncate">{opt.label}</span>
              {#if opt.hint}<span class="shrink-0 font-mono text-[10.5px] text-muted">{opt.hint}</span>{/if}
            </button>
          {/each}
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .dd-trigger.dd-open {
    outline: none;
    border-color: var(--accent-ink);
    box-shadow: 0 0 0 3px var(--accent-dim);
  }
</style>
