<script lang="ts">
  import { onMount } from 'svelte';
  import { monitor } from '../monitor.svelte';
  import { store } from '../store.svelte';
  import { api, type ApiKey, type Model, type MonitorRange, type MonitorTotals } from '../api';
  import { reveal } from '../motion.svelte';
  import { compact, latency, int, pct, delta } from '../format';
  import Dropdown, { type DropdownOption } from '../components/Dropdown.svelte';
  import LiveFeed from '../components/monitor/LiveFeed.svelte';
  import ServingNow from '../components/monitor/ServingNow.svelte';
  import UsageChart from '../components/monitor/UsageChart.svelte';
  import HistoryTable from '../components/monitor/HistoryTable.svelte';

  const RANGES: { key: MonitorRange; label: string }[] = [
    { key: 'daily', label: 'Daily' },
    { key: 'monthly', label: 'Monthly' },
    { key: 'yearly', label: 'Yearly' },
  ];

  let keys = $state<ApiKey[]>([]);
  let models = $state<Model[]>([]);

  onMount(() => {
    monitor.refreshHistorical();
    monitor.refreshHistory(1);
    monitor.connect();
    api
      .keys()
      .then((ks) => (keys = ks ?? []))
      .catch(() => {});
    api
      .models()
      .then((ms) => (models = ms ?? []))
      .catch(() => {});
    return () => monitor.disconnect();
  });

  const modelOptions = $derived([...new Set(models.map((m) => m.public_id))].sort());
  const providerOptions = $derived([...new Set(store.providers.map((p) => p.name))].sort());
  const groupOptions = $derived([...new Set(store.groups.map((g) => g.name))].sort());

  const names = (list: string[]): DropdownOption[] => list.map((n) => ({ value: n, label: n }));
  const keyOptions = $derived(keys.map((k) => ({ value: String(k.id), label: k.name })) as DropdownOption[]);

  const hasFilters = $derived(
    Boolean(
      monitor.filters.keyId ||
        monitor.filters.provider ||
        monitor.filters.model ||
        monitor.filters.protocol ||
        monitor.filters.group ||
        monitor.filters.outcome ||
        monitor.filters.stream,
    ),
  );

  function clearFilters() {
    monitor.setFilter({
      keyId: '',
      provider: '',
      model: '',
      protocol: '',
      group: '',
      outcome: '',
      stream: '',
    });
  }

  interface Tile {
    label: string;
    value: string;
    sub?: string;
    deltaText: string;
    neutral: boolean;
    direction: number;
    title: string;
  }

  const MIN_SAMPLE = 10;

  function tile(
    label: string,
    cur: number | null | undefined,
    prev: number | null | undefined,
    fmt: (v: number) => string,
    direction: number,
    title: string,
    sampled: boolean,
  ): Tile {
    const d = delta(cur, prev);
    return {
      label,
      value: cur === null || cur === undefined ? '—' : fmt(cur),
      deltaText: sampled ? (d === '' ? '' : `${d} vs prev`) : 'low sample',
      neutral: !sampled,
      direction,
      title,
    };
  }

  function ppDiff(cur: number | null | undefined, prev: number | null | undefined): string {
    if (cur === null || cur === undefined || prev === null || prev === undefined) return '';
    const diff = (cur - prev) * 100;
    if (Math.abs(diff) < 0.05) return '±0pp vs prev';
    return `${diff > 0 ? '+' : ''}${diff.toFixed(1)}pp vs prev`;
  }

  const tiles = $derived.by((): Tile[] => {
    const s = monitor.summary;
    if (!s) return [];
    const c: MonitorTotals = s.current;
    const p: MonitorTotals = s.previous;
    const sampled = p.requests >= MIN_SAMPLE;
    const rateTile: Tile = {
      label: 'Success Rate',
      value: pct(c.success_rate),
      sub: c.cancelled > 0 ? `${int(c.cancelled)} cancelled excluded` : undefined,
      deltaText: sampled ? ppDiff(c.success_rate, p.success_rate) : 'low sample',
      neutral: !sampled,
      direction: 1,
      title: 'Succeeded ÷ (requests − cancelled)',
    };
    return [
      tile('Requests', c.requests ?? null, p.requests ?? null, int, 1, 'Inbound requests this period', sampled),
      tile('Input Tokens', c.input_tokens, p.input_tokens, compact, 1, 'Prompt tokens reported by upstream providers', sampled),
      tile('Output Tokens', c.output_tokens, p.output_tokens, compact, 1, 'Completion tokens reported by upstream providers', sampled),
      tile('Avg Latency', c.avg_latency_ms, p.avg_latency_ms, (v) => latency(v), -1, 'Mean total request duration', sampled),
      rateTile,
    ];
  });

  function deltaClass(t: Tile): string {
    if (!t.deltaText || t.neutral || t.deltaText.startsWith('±')) return 'text-tertiary';
    const up = t.deltaText.startsWith('+');
    const good = t.direction * (up ? 1 : -1) > 0;
    return good ? 'text-success-muted' : 'text-error-muted';
  }
</script>

<header use:reveal={{ kind: 'rise', i: 0 }} class="mb-8 flex flex-wrap items-end justify-between gap-x-5 gap-y-3">
  <div>
    <p class="mb-4 font-mono text-[11px] tracking-[0.1em] text-accent-ink uppercase">Management</p>
    <h1 class="leading-none">Monitoring</h1>
    <p class="mt-2.5 text-sm text-tertiary">Live activity and usage across pogu.</p>
  </div>
  <div use:reveal={{ kind: 'pop', i: 1 }} class="flex flex-col items-end gap-1" role="group" aria-label="Historical time range">
    <div class="flex gap-0.5 rounded-sm border border-line bg-well p-0.5">
      {#each RANGES as r (r.key)}
        <button
          aria-pressed={monitor.range === r.key}
          class="rounded-[2px] px-3 py-1.5 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {monitor.range === r.key
            ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
            : 'text-tertiary hover:text-primary'}"
          onclick={() => monitor.setRange(r.key)}
        >
          {r.label}
        </button>
      {/each}
    </div>
    <p class="m-0 font-mono text-[10px] tracking-[0.07em] text-tertiary uppercase">historical range</p>
  </div>
</header>

{#if monitor.loadError}
  <p use:reveal={{ kind: 'blip' }} class="error-line mb-6">{monitor.loadError}</p>
{/if}

{#snippet LoadFailed(onretry: () => void)}
  <div use:reveal={{ kind: 'blip' }} class="flex flex-col items-center rounded-sm border border-dashed border-warning-soft px-5 py-10 text-center">
    <p class="m-0 text-[13px] text-warning-muted">Couldn't load monitoring data.</p>
    <p class="mt-1.5 mb-4 max-w-[56ch] text-xs text-tertiary">
      Check that pogu is running, then retry.
    </p>
    <button class="btn btn-sm" onclick={onretry}>Retry</button>
  </div>
{/snippet}

<div use:reveal={{ kind: 'rise', i: 1 }} class="mb-5 flex flex-wrap items-center gap-x-2.5 gap-y-2">
  <span class="mr-1 font-mono text-[10px] tracking-[0.07em] text-tertiary uppercase">Filter</span>
  <Dropdown
    size="sm"
    class="w-36"
    label="API key"
    placeholder="All keys"
    searchable
    searchPlaceholder="Search keys…"
    emptyText="No keys match"
    options={keyOptions}
    value={monitor.filters.keyId}
    onchange={(v) => monitor.setFilter({ keyId: v })}
  />
  <Dropdown
    size="sm"
    class="w-36"
    label="Provider"
    placeholder="All providers"
    searchable
    searchPlaceholder="Search providers…"
    emptyText="No providers match"
    options={names(providerOptions)}
    value={monitor.filters.provider}
    onchange={(v) => monitor.setFilter({ provider: v })}
  />
  <Dropdown
    size="sm"
    class="w-36"
    label="Model"
    placeholder="All models"
    searchable
    searchPlaceholder="Search models…"
    emptyText="No models match"
    options={names(modelOptions)}
    value={monitor.filters.model}
    onchange={(v) => monitor.setFilter({ model: v })}
  />
  <Dropdown
    size="sm"
    class="w-36"
    label="Protocol"
    placeholder="Both protocols"
    options={[
      { value: 'openai', label: 'OpenAI' },
      { value: 'anthropic', label: 'Anthropic' },
    ]}
    value={monitor.filters.protocol}
    onchange={(v) => monitor.setFilter({ protocol: v as '' | 'openai' | 'anthropic' })}
  />
  <Dropdown
    size="sm"
    class="w-36"
    label="Group"
    placeholder="All groups"
    searchable
    searchPlaceholder="Search groups…"
    emptyText="No groups match"
    options={names(groupOptions)}
    value={monitor.filters.group}
    onchange={(v) => monitor.setFilter({ group: v })}
  />
  <Dropdown
    size="sm"
    class="w-36"
    label="Status"
    placeholder="All statuses"
    options={[
      { value: 'success', label: 'Success' },
      { value: 'failed', label: 'Failed' },
      { value: 'cancelled', label: 'Cancelled' },
    ]}
    value={monitor.filters.outcome}
    onchange={(v) => monitor.setFilter({ outcome: v as '' | 'success' | 'failed' | 'cancelled' })}
  />
  <Dropdown
    size="sm"
    class="w-36"
    label="Streaming"
    placeholder="Stream & unary"
    options={[
      { value: 'true', label: 'Streaming only' },
      { value: 'false', label: 'Non-streaming only' },
    ]}
    value={monitor.filters.stream}
    onchange={(v) => monitor.setFilter({ stream: v as '' | 'true' | 'false' })}
  />
  {#if hasFilters}
    <button
      class="linkish"
      onclick={clearFilters}
    >
      Clear
    </button>
  {/if}
</div>

{#if !monitor.loaded}
  <div class="grid animate-pulse grid-cols-2 gap-px overflow-hidden rounded-sm border border-line bg-line sm:grid-cols-3 lg:grid-cols-5" aria-hidden="true">
    {#each Array(5) as _, i (i)}
      <div class="bg-raised px-4 py-4">
        <div class="mb-2 h-2 w-14 rounded-full bg-well"></div>
        <div class="h-7 w-20 rounded-sm bg-well"></div>
      </div>
    {/each}
  </div>
{:else if monitor.summary}
  <dl class="m-0 grid grid-cols-2 gap-px overflow-hidden rounded-sm border border-line bg-line sm:grid-cols-3 lg:grid-cols-5">
    {#each tiles as t, i (t.label)}
      <div use:reveal={{ kind: 'blip', i: i + 1 }} class="min-w-0 bg-raised px-4 py-3.5" title={t.title}>
        <dt class="mb-1 font-mono text-[10px] font-semibold tracking-[0.07em] whitespace-nowrap text-tertiary uppercase">{t.label}</dt>
        <dd class="m-0 truncate font-heading text-[26px] leading-tight font-semibold tracking-[-0.01em] text-paper tabular-nums">
          {t.value}
        </dd>
        <p class="m-0 mt-0.5 min-h-[15px] font-mono text-[10px] {deltaClass(t)}">
          {t.deltaText}{#if t.sub}<span class="ml-2 text-tertiary">{t.sub}</span>{/if}
        </p>
      </div>
    {/each}
  </dl>
{:else}
  {@render LoadFailed(() => monitor.refreshHistorical())}
{/if}

<section use:reveal={{ kind: 'rise', i: 2 }} aria-labelledby="live-h" class="mt-[42px]">
  <div class="sec-head">
    <h2 id="live-h">Right now</h2>
    <p class="m-0 font-mono text-[10px] tracking-[0.07em] text-tertiary uppercase">
      live regardless of the selected range
    </p>
  </div>
  <div class="grid items-start gap-3 wide:grid-cols-2">
    <LiveFeed />
    <ServingNow />
  </div>
</section>

<section use:reveal={{ kind: 'rise', i: 3 }} aria-labelledby="usage-chart-h" class="mt-[42px]" aria-label="Historical usage">
  <UsageChart />
</section>

<HistoryTable />
