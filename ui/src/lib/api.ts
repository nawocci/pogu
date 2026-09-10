export type ProviderType = 'openai' | 'openai-responses' | 'anthropic';
export type KeySelection = 'first' | 'round_robin';

export interface Provider {
  id: number;
  name: string;
  type: ProviderType;
  prefix: string;
  base_url: string;
  has_api_key: boolean;
  key_count: number;
  key_selection: KeySelection;
  enabled: boolean;
  builtin: string;
  created_at: string;
  updated_at: string;
}

export interface ProviderKey {
  id: number;
  provider_id: number;
  name: string;
  masked_key: string;
  enabled: boolean;
  sort_order: number;
  last_used_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface Model {
  id: number;
  provider: string;
  public_id: string;
  provider_id: number;
  name: string;
  enabled: boolean;
  updated_at: string;
}

export interface ApiKey {
  id: number;
  name: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

export interface ProviderInput {
  name: string;
  type: ProviderType;
  prefix: string;
  base_url: string;
  key_selection?: KeySelection;
  api_key?: string;
  enabled: boolean;
}

export interface ProviderKeyInput {
  name?: string;
  secret: string;
}

export interface ProviderKeyUpdateInput {
  name: string;
  enabled: boolean;
}

export interface ModelInput {
  provider_id: number;
  name: string;
  enabled: boolean;
}

export interface Group {
  id: number;
  name: string;
  selection: KeySelection;
  enabled: boolean;
  member_count: number;
  created_at: string;
  updated_at: string;
}

export interface GroupMember {
  id: number;
  group_id: number;
  model_id: number;
  position: number;
  enabled: boolean;
  public_id: string;
  provider: string;
  prefix: string;
  model_name: string;
  created_at: string;
  updated_at: string;
}

export interface GroupInput {
  name: string;
  selection: KeySelection;
  enabled: boolean;
}

export function groupInput(g: Group, overrides: Partial<GroupInput> = {}): GroupInput {
  return { name: g.name, selection: g.selection, enabled: g.enabled, ...overrides };
}

export interface ImportEntry {
  line_number: number;
  name: string;
  masked_key: string;
  valid: boolean;
  error?: string;
}

export interface ImportResult {
  total: number;
  valid: number;
  invalid: number;
  entries: ImportEntry[];
}

export type MonitorRange = 'daily' | 'monthly' | 'yearly';
export type MonitorOutcome = 'success' | 'failed' | 'cancelled';

export interface MonitorFilters {
  range: MonitorRange;
  keyId: string;
  provider: string;
  model: string;
  protocol: '' | ProviderType;
  group: string;
  outcome: '' | MonitorOutcome;
  stream: '' | 'true' | 'false';
}

export interface MonitorAttempt {
  number: number;
  provider_name: string;
  provider_prefix: string;
  provider_type: string;
  upstream_model: string;
  credential_id: number | null;
  http_status: number | null;
  success: boolean;
  error_category: string;
  duration_ms: number | null;
  input_tokens: number | null;
  output_tokens: number | null;
  total_tokens: number | null;
}

export interface MonitorRecord {
  request_id: string;
  protocol: string;
  ts: string;
  key_id: number | null;
  key_name: string | null;
  model: string;
  resolved_model: string | null;
  served_model: string | null;
  group_id: number | null;
  group_name: string | null;
  provider: string;
  http_status: number | null;
  error: string;
  cancelled: boolean;
  stream: boolean;
  input_tokens: number | null;
  output_tokens: number | null;
  total_tokens: number | null;
  latency_ms: number | null;
  ttft_ms: number | null;
  upstream_ms: number | null;
  attempts: MonitorAttempt[];
}

export interface MonitorStarted {
  request_id: string;
  ts: string;
  key_id?: number | null;
  key_name?: string | null;
  model: string;
  protocol: string;
  stream: boolean;
  provider?: string;
  upstream_model?: string;
  resolved_model?: string;
  group_name?: string;
}

export interface MonitorTotals {
  requests: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  input_tokens: number | null;
  output_tokens: number | null;
  avg_latency_ms: number | null;
  success_rate: number | null;
}

export interface SummaryResult {
  range: MonitorRange;
  start: string;
  end: string;
  interval: 'hour' | 'day' | 'month';
  current: MonitorTotals;
  previous: MonitorTotals;
}

export interface MonitorBucket {
  t: string;
  requests: number;
  succeeded: number;
  failed: number;
  cancelled: number;
  input_tokens: number | null;
  output_tokens: number | null;
  avg_latency_ms: number | null;
}

export interface RequestPage {
  items: MonitorRecord[];
  total: number;
  page: number;
  page_size: number;
}

export const MONITORING_EVENTS_URL = '/api/monitoring/events';

export function monitorQuery(f: MonitorFilters): string {
  const p = new URLSearchParams({ range: f.range });
  if (f.keyId) p.set('key_id', f.keyId);
  if (f.provider) p.set('provider', f.provider);
  if (f.model) p.set('model', f.model);
  if (f.protocol) p.set('protocol', f.protocol);
  if (f.group) p.set('group', f.group);
  if (f.outcome) p.set('status', f.outcome);
  if (f.stream) p.set('stream', f.stream);
  return p.toString();
}

export function providerInput(p: Provider, overrides: Partial<ProviderInput> = {}): ProviderInput {
  return {
    name: p.name,
    type: p.type,
    prefix: p.prefix,
    base_url: p.base_url,
    key_selection: p.key_selection,
    enabled: p.enabled,
    ...overrides,
  };
}

export interface TestResult {
  ok?: boolean;
  message?: string;
  models?: string[];
}

export interface OpenCodeSyncSummary {
  catalog: number;
  free: number;
  added: number;
  disabled: number;
}

export interface CreatedKeyResponse {
  key: ApiKey;
  secret: string;
}

export interface CreatedProviderKeyResponse {
  key: ProviderKey;
  secret: string;
}

export interface GlobalPrompt {
  system_prompt: string;
  enabled: boolean;
}

export interface CavemanLevelMeta {
  id: string;
  label: string;
  description: string;
}

export interface CavemanSettings {
  enabled: boolean;
  level: string;
  last_synced_at: string;
  levels: CavemanLevelMeta[];
}

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export type UnauthorizedHandler = () => void;

let onUnauthorized: UnauthorizedHandler | null = null;

export function setUnauthorizedHandler(handler: UnauthorizedHandler): void {
  onUnauthorized = handler;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', ...(options.headers ?? {}) },
    ...options,
  });
  let data: unknown = null;
  try {
    data = await response.json();
  } catch {
    // empty response body
  }
  if (!response.ok) {
    if (response.status === 401 && path !== '/api/auth/login' && onUnauthorized) {
      onUnauthorized();
    }
    const record = data as { error?: { message?: string }; message?: string } | null;
    throw new ApiError(
      response.status,
      record?.error?.message ?? record?.message ?? `Request failed (${response.status})`,
    );
  }
  return data as T;
}

export const api = {
  login: (password: string) =>
    request<{ authenticated: boolean }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),
  me: () => request<{ authenticated: boolean }>('/api/auth/me'),
  setupStatus: () => request<{ setup_required: boolean }>('/api/auth/setup'),
  completeSetup: (token: string, password: string) =>
    request<{ authenticated: boolean }>('/api/auth/setup', {
      method: 'POST',
      body: JSON.stringify({ token, password }),
    }),
  changePassword: (current_password: string, new_password: string) =>
    request<{ changed: boolean }>('/api/auth/password', {
      method: 'POST',
      body: JSON.stringify({ current_password, new_password }),
    }),
  providers: () => request<Provider[]>('/api/providers'),
  getProvider: (id: number) => request<Provider>(`/api/providers/${id}`),
  createProvider: (input: ProviderInput) =>
    request<Provider>('/api/providers', { method: 'POST', body: JSON.stringify(input) }),
  updateProvider: (id: number, input: ProviderInput) =>
    request<Provider>(`/api/providers/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteProvider: (id: number) => request<void>(`/api/providers/${id}`, { method: 'DELETE' }),
  testProvider: (id: number) =>
    request<TestResult>(`/api/providers/${id}/test`, { method: 'POST' }),
  syncProvider: (id: number) =>
    request<OpenCodeSyncSummary>(`/api/providers/${id}/sync`, { method: 'POST' }),

  providerKeys: (providerId: number) =>
    request<ProviderKey[]>(`/api/providers/${providerId}/keys`),
  createProviderKey: (providerId: number, input: ProviderKeyInput) =>
    request<CreatedProviderKeyResponse>(`/api/providers/${providerId}/keys`, {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  importProviderKeys: (providerId: number, text: string, preview: boolean) =>
    request<ImportResult>(`/api/providers/${providerId}/keys/import`, {
      method: 'POST',
      body: JSON.stringify({ text, preview }),
    }),
  updateProviderKey: (providerId: number, keyId: number, input: ProviderKeyUpdateInput) =>
    request<ProviderKey>(`/api/providers/${providerId}/keys/${keyId}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  deleteProviderKey: (providerId: number, keyId: number) =>
    request<void>(`/api/providers/${providerId}/keys/${keyId}`, { method: 'DELETE' }),
  makeProviderKeyPrimary: (providerId: number, keyId: number) =>
    request<ProviderKey[]>(`/api/providers/${providerId}/keys/${keyId}/primary`, {
      method: 'POST',
    }),
  testProviderKey: (providerId: number, keyId: number) =>
    request<TestResult>(`/api/providers/${providerId}/keys/${keyId}/test`, {
      method: 'POST',
    }),

  models: () => request<Model[]>('/api/models'),
  createModel: (input: ModelInput) =>
    request<Model>('/api/models', { method: 'POST', body: JSON.stringify(input) }),
  updateModel: (id: number, input: ModelInput) =>
    request<Model>(`/api/models/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteModel: (id: number) => request<void>(`/api/models/${id}`, { method: 'DELETE' }),

  groups: () => request<Group[]>('/api/groups'),
  getGroup: (id: number) => request<Group>(`/api/groups/${id}`),
  createGroup: (input: GroupInput) =>
    request<Group>('/api/groups', { method: 'POST', body: JSON.stringify(input) }),
  updateGroup: (id: number, input: GroupInput) =>
    request<Group>(`/api/groups/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteGroup: (id: number) => request<void>(`/api/groups/${id}`, { method: 'DELETE' }),
  groupMembers: (id: number) => request<GroupMember[]>(`/api/groups/${id}/members`),
  addGroupMember: (id: number, modelId: number) =>
    request<GroupMember>(`/api/groups/${id}/members`, {
      method: 'POST',
      body: JSON.stringify({ model_id: modelId }),
    }),
  updateGroupMember: (id: number, memberId: number, enabled: boolean) =>
    request<GroupMember>(`/api/groups/${id}/members/${memberId}`, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    }),
  deleteGroupMember: (id: number, memberId: number) =>
    request<void>(`/api/groups/${id}/members/${memberId}`, { method: 'DELETE' }),
  reorderGroupMembers: (id: number, memberIds: number[]) =>
    request<GroupMember[]>(`/api/groups/${id}/members/order`, {
      method: 'POST',
      body: JSON.stringify({ member_ids: memberIds }),
    }),
  setGroupSelection: (id: number, selection: KeySelection) =>
    request<Group>(`/api/groups/${id}/selection`, {
      method: 'POST',
      body: JSON.stringify({ selection }),
    }),

  keys: () => request<ApiKey[]>('/api/keys'),
  createKey: (name: string) =>
    request<CreatedKeyResponse>('/api/keys', { method: 'POST', body: JSON.stringify({ name }) }),
  revokeKey: (id: number) => request<void>(`/api/keys/${id}/revoke`, { method: 'POST' }),

  globalPrompt: () => request<GlobalPrompt>('/api/settings/prompt'),
  updateGlobalPrompt: (input: GlobalPrompt) =>
    request<GlobalPrompt>('/api/settings/prompt', { method: 'PUT', body: JSON.stringify(input) }),

  cavemanSettings: () => request<CavemanSettings>('/api/settings/caveman'),
  updateCavemanSettings: (input: { enabled: boolean; level: string }) =>
    request<CavemanSettings>('/api/settings/caveman', { method: 'PUT', body: JSON.stringify(input) }),
  syncCavemanSkill: () =>
    request<CavemanSettings>('/api/settings/caveman/sync', { method: 'POST' }),

  monitoringSummary: (f: MonitorFilters) =>
    request<SummaryResult>(`/api/monitoring/summary?${monitorQuery(f)}`),
  monitoringTimeseries: (f: MonitorFilters) =>
    request<{ buckets: MonitorBucket[] }>(`/api/monitoring/timeseries?${monitorQuery(f)}`),
  monitoringRequests: (f: MonitorFilters, page: number, pageSize = 25) =>
    request<RequestPage>(
      `/api/monitoring/requests?${monitorQuery(f)}&page=${page}&page_size=${pageSize}`,
    ),
};
