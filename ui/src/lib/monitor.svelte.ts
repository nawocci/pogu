import {
  api,
  MONITORING_EVENTS_URL,
  type MonitorFilters,
  type MonitorRange,
  type MonitorRecord,
  type MonitorStarted,
  type SummaryResult,
  type MonitorBucket,
  type RequestPage,
} from './api';

export type FeedState = 'active' | 'streaming' | 'completed' | 'failed' | 'cancelled';

export interface FeedEntry {
  requestId: string;
  ts: number;
  model: string;
  resolvedModel?: string;
  servedModel?: string;
  upstreamModel?: string;
  groupName?: string;
  provider: string;
  protocol: string;
  keyLabel: string;
  stream: boolean;
  state: FeedState;
  status?: number;
  errType?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  latencyMs?: number;
  ttftMs?: number;
  upstreamMs?: number;
}

export interface RouteLane {
  id: string;
  model: string;
  resolved?: string;
  served?: string;
  upstream?: string;
  provider: string;
  callers: { label: string }[];
  count: number;
  failed: number;
  cancelled: number;
  active: boolean;
  firstTs: number;
}

const LANE_CAP = 8;
const FEED_CAP = 120;

function keyLabelFor(rec: { key_id?: number | null; key_name?: string | null }): string {
  if (rec.key_name) return rec.key_name;
  if (rec.key_id != null) return 'Removed key';
  return 'Unknown key';
}

interface StartedLike {
  request_id: string;
  ts: string;
  key_id?: number | null;
  key_name?: string | null;
  model: string;
  protocol: string;
  stream: boolean;
  provider?: string | null;
  upstream_model?: string | null;
  resolved_model?: string | null;
  served_model?: string | null;
  group_name?: string | null;
}

function entryFromStarted(ev: StartedLike): FeedEntry {
  return {
    requestId: ev.request_id,
    ts: new Date(ev.ts).getTime(),
    model: ev.model,
    resolvedModel: ev.resolved_model ?? undefined,
    servedModel: ev.served_model ?? undefined,
    upstreamModel: ev.upstream_model ?? undefined,
    groupName: ev.group_name ?? undefined,
    provider: ev.provider ?? '',
    protocol: ev.protocol,
    keyLabel: keyLabelFor(ev),
    stream: ev.stream,
    state: 'active',
  };
}

function entryFromRecord(rec: MonitorRecord): FeedEntry {
  const entry = entryFromStarted(rec);
  entry.ts = new Date(rec.ts).getTime();
  return applyFinished(entry, rec);
}

function applyFinished(entry: FeedEntry, rec: MonitorRecord): FeedEntry {
  entry.keyLabel = keyLabelFor(rec);
  if (rec.provider) entry.provider = rec.provider;
  if (rec.resolved_model) entry.resolvedModel = rec.resolved_model;
  if (rec.served_model) entry.servedModel = rec.served_model;
  if (rec.group_name) entry.groupName = rec.group_name;
  entry.status = rec.http_status ?? undefined;
  entry.errType = rec.error || undefined;
  entry.inputTokens = rec.input_tokens ?? undefined;
  entry.outputTokens = rec.output_tokens ?? undefined;
  entry.totalTokens = rec.total_tokens ?? undefined;
  entry.latencyMs = rec.latency_ms ?? undefined;
  entry.ttftMs = rec.ttft_ms ?? undefined;
  entry.upstreamMs = rec.upstream_ms ?? undefined;
  entry.state = rec.cancelled ? 'cancelled' : rec.http_status != null && rec.http_status >= 400 || rec.error ? 'failed' : 'completed';
  return entry;
}

export function emptyFilters(range: MonitorRange): MonitorFilters {
  return { range, keyId: '', provider: '', model: '', protocol: '', group: '', outcome: '', stream: '' };
}

class MonitoringStore {
  range = $state<MonitorRange>('daily');
  filters = $state<MonitorFilters>(emptyFilters('daily'));

  loaded = $state(false);
  loadError = $state('');
  summary = $state<SummaryResult | null>(null);
  buckets = $state<MonitorBucket[]>([]);
  history = $state<RequestPage | null>(null);
  page = $state(1);
  historyLoading = $state(false);

  feed = $state<FeedEntry[]>([]);
  liveConnected = $state(false);

  #es: EventSource | null = null;
  #dirty = false;
  #timer: ReturnType<typeof setInterval> | null = null;

  #histGen = 0;
  #pageGen = 0;

  get activeEntries(): FeedEntry[] {
    return this.feed.filter((e) => e.state === 'active' || e.state === 'streaming');
  }

  get finishedEntries(): FeedEntry[] {
    return this.feed.filter((e) => e.state !== 'active' && e.state !== 'streaming');
  }

  get laneView(): { lanes: RouteLane[]; overflow: number } {
    const map = new Map<string, RouteLane>();
    const callersByLane = new Map<string, Map<string, { label: string }>>();
    let overflow = 0;

    const entries = [...this.feed].sort((a, b) => b.ts - a.ts);
    for (const e of entries) {
      if (!e.provider) continue;
      const id = `${e.model}\0${e.resolvedModel ?? ''}\0${e.provider}`;
      let lane = map.get(id);
      if (!lane) {
        if (map.size >= LANE_CAP && !activeState(e.state)) {
          overflow++;
          continue;
        }
        lane = {
          id,
          model: e.model,
          resolved: e.resolvedModel || undefined,
          served: e.servedModel || undefined,
          upstream: e.upstreamModel || undefined,
          provider: e.provider,
          callers: [],
          count: 0,
          failed: 0,
          cancelled: 0,
          active: activeState(e.state),
          firstTs: e.ts,
        };
        map.set(id, lane);
        callersByLane.set(id, new Map());
      }
      lane.count++;
      if (e.state === 'failed') lane.failed++;
      else if (e.state === 'cancelled') lane.cancelled++;
      if (activeState(e.state)) lane.active = true;
      if (e.resolvedModel && !lane.resolved) lane.resolved = e.resolvedModel;
      if (e.servedModel) lane.served = e.servedModel;
      if (e.upstreamModel) lane.upstream = e.upstreamModel;
      lane.firstTs = Math.min(lane.firstTs, e.ts);

      const callers = callersByLane.get(id)!;
      if (!callers.has(e.keyLabel)) callers.set(e.keyLabel, { label: e.keyLabel });
    }

    const lanes = [...map.values()];
    for (const lane of lanes) lane.callers = [...callersByLane.get(lane.id)!.values()];
    lanes.sort(
      (a, b) =>
        Number(b.active) - Number(a.active) ||
        b.count - a.count ||
        a.firstTs - b.firstTs ||
        a.id.localeCompare(b.id),
    );
    overflow += Math.max(0, lanes.length - LANE_CAP);
    return { lanes: lanes.slice(0, LANE_CAP), overflow };
  }

  async refreshHistorical() {
    const gen = ++this.#histGen;
    try {
      const [summary, series] = await Promise.all([
        api.monitoringSummary(this.filters),
        api.monitoringTimeseries(this.filters),
      ]);
      if (gen !== this.#histGen) return;
      this.summary = summary;
      this.buckets = series.buckets;
      this.loadError = '';
    } catch (e) {
      if (gen !== this.#histGen) return;
      this.loadError = e instanceof Error ? e.message : String(e);
    } finally {
      this.loaded = true;
    }
  }

  async refreshHistory(page = this.page) {
    const gen = ++this.#pageGen;
    this.historyLoading = true;
    try {
      const result = await api.monitoringRequests(this.filters, page);
      if (gen !== this.#pageGen) return;
      this.history = result;
      this.page = page;
    } catch (e) {
      if (gen !== this.#pageGen) return;
      this.loadError = e instanceof Error ? e.message : String(e);
    } finally {
      if (gen === this.#pageGen) this.historyLoading = false;
    }
  }

  setRange(range: MonitorRange) {
    if (this.range === range) return;
    this.range = range;
    this.filters = { ...this.filters, range };
    this.page = 1;
    this.refreshHistorical();
    this.refreshHistory(1);
  }

  setFilter(patch: Partial<MonitorFilters>) {
    this.filters = { ...this.filters, ...patch };
    this.page = 1;
    this.refreshHistorical();
    this.refreshHistory(1);
  }

  connect() {
    if (this.#es) return;
    const es = new EventSource(MONITORING_EVENTS_URL);
    this.#es = es;

    es.onopen = () => (this.liveConnected = true);
    es.onerror = () => (this.liveConnected = false);
    es.addEventListener('hello', (ev) => {
      this.liveConnected = true;
      try {
        const data = JSON.parse((ev as MessageEvent).data) as {
          active: MonitorStarted[];
          recent: MonitorRecord[];
        };
        const active = data.active.map(entryFromStarted);
        const seeded = data.recent.map(entryFromRecord);
        this.feed = [...active, ...seeded].slice(0, FEED_CAP);
      } catch {
        /* malformed hello: the next events will still populate */
      }
    });
    es.addEventListener('request_started', (ev) => {
      try {
        const started = JSON.parse((ev as MessageEvent).data) as MonitorStarted;
        if (this.feed.some((e) => e.requestId === started.request_id)) return;
        this.feed.unshift(entryFromStarted(started));
        if (this.feed.length > FEED_CAP) this.feed.length = FEED_CAP;
        this.#dirty = true;
      } catch {
        /* ignore malformed frame */
      }
    });
    es.addEventListener('request_routed', (ev) => {
      try {
        const d = JSON.parse((ev as MessageEvent).data) as {
          request_id: string;
          provider?: string;
          upstream_model?: string;
          resolved_model?: string;
          group_name?: string;
        };
        const entry = this.feed.find((e) => e.requestId === d.request_id);
        if (!entry) return;
        if (d.provider) entry.provider = d.provider;
        if (d.upstream_model) entry.upstreamModel = d.upstream_model;
        if (d.resolved_model) entry.resolvedModel = d.resolved_model;
        if (d.group_name) entry.groupName = d.group_name;
      } catch {
        /* ignore malformed frame */
      }
    });
    es.addEventListener('request_streaming', (ev) => {
      try {
        const d = JSON.parse((ev as MessageEvent).data) as {
          request_id: string;
          ttft_ms?: number;
          served_model?: string;
        };
        const entry = this.feed.find((e) => e.requestId === d.request_id);
        if (!entry) return;
        if (d.ttft_ms !== undefined) entry.ttftMs = d.ttft_ms;
        if (d.served_model) entry.servedModel = d.served_model;
        if (entry.state === 'active') entry.state = 'streaming';
      } catch {
        /* ignore malformed frame */
      }
    });
    es.addEventListener('request_finished', (ev) => {
      try {
        const rec = JSON.parse((ev as MessageEvent).data) as MonitorRecord;
        const entry = this.feed.find((e) => e.requestId === rec.request_id);
        if (entry) {
          applyFinished(entry, rec);
          this.feed = [
            ...this.feed.filter((e) => e !== entry && (e.state === 'active' || e.state === 'streaming')),
            entry,
            ...this.feed.filter((e) => e !== entry && e.state !== 'active' && e.state !== 'streaming'),
          ];
        } else {
          const fresh = entryFromRecord(rec);
          const firstFinished = this.feed.findIndex(
            (e) => e.state !== 'active' && e.state !== 'streaming',
          );
          if (firstFinished === -1) this.feed.push(fresh);
          else this.feed.splice(firstFinished, 0, fresh);
        }
        this.#dirty = true;
      } catch {
        /* ignore malformed frame */
      }
    });

    this.#timer = setInterval(() => {
      if (this.#dirty && document.visibilityState !== 'hidden') {
        this.#dirty = false;
        this.refreshHistorical();
      }
    }, 4000);
  }

  disconnect() {
    this.#es?.close();
    this.#es = null;
    this.liveConnected = false;
    if (this.#timer) clearInterval(this.#timer);
    this.#timer = null;
  }
}

function activeState(s: FeedState): boolean {
  return s === 'active' || s === 'streaming';
}

export const monitor = new MonitoringStore();
