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

export interface CreatedKeyResponse {
  key: ApiKey;
  secret: string;
}

export interface CreatedProviderKeyResponse {
  key: ProviderKey;
  secret: string;
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
  providers: () => request<Provider[]>('/api/providers'),
  getProvider: (id: number) => request<Provider>(`/api/providers/${id}`),
  createProvider: (input: ProviderInput) =>
    request<Provider>('/api/providers', { method: 'POST', body: JSON.stringify(input) }),
  updateProvider: (id: number, input: ProviderInput) =>
    request<Provider>(`/api/providers/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteProvider: (id: number) => request<void>(`/api/providers/${id}`, { method: 'DELETE' }),
  testProvider: (id: number) =>
    request<TestResult>(`/api/providers/${id}/test`, { method: 'POST' }),

  providerKeys: (providerId: number) =>
    request<ProviderKey[]>(`/api/providers/${providerId}/keys`),
  createProviderKey: (providerId: number, input: ProviderKeyInput) =>
    request<CreatedProviderKeyResponse>(`/api/providers/${providerId}/keys`, {
      method: 'POST',
      body: JSON.stringify(input),
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

  keys: () => request<ApiKey[]>('/api/keys'),
  createKey: (name: string) =>
    request<CreatedKeyResponse>('/api/keys', { method: 'POST', body: JSON.stringify({ name }) }),
  revokeKey: (id: number) => request<void>(`/api/keys/${id}/revoke`, { method: 'POST' }),
};
