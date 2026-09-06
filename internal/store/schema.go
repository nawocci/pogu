package store

const schemaProviders = `
CREATE TABLE IF NOT EXISTS providers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('openai','openai-responses','anthropic')),
  prefix TEXT NOT NULL UNIQUE,
  base_url TEXT NOT NULL,
  key_selection TEXT NOT NULL DEFAULT 'first' CHECK (key_selection IN ('first','round_robin')),
  enabled INTEGER NOT NULL DEFAULT 1,
  builtin TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
)`

const schemaProviderKeys = `
CREATE TABLE IF NOT EXISTS provider_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  encrypted_secret TEXT NOT NULL,
  secret_hash BLOB NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  last_used_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (provider_id, secret_hash)
);
CREATE INDEX IF NOT EXISTS idx_provider_keys_provider ON provider_keys(provider_id, sort_order, id)`

const schemaModels = `
CREATE TABLE IF NOT EXISTS models (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  scheme TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (provider_id, name)
);
CREATE INDEX IF NOT EXISTS idx_models_provider ON models(provider_id)`

const schemaClients = `
CREATE TABLE IF NOT EXISTS client_keys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  key_hash BLOB NOT NULL UNIQUE,
  last_used_at TEXT,
  expires_at TEXT,
  revoked_at TEXT,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS admin (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token_hash BLOB PRIMARY KEY,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at)`

const schemaTelemetry = `
CREATE TABLE IF NOT EXISTS telemetry_requests (
  id TEXT PRIMARY KEY,
  protocol TEXT NOT NULL CHECK (protocol IN ('openai','anthropic')),
  public_model TEXT NOT NULL,
  resolved_model TEXT,
  served_model TEXT,
  group_id INTEGER,
  group_name TEXT,
  streaming INTEGER NOT NULL DEFAULT 0,
  client_key_id INTEGER,
  client_key_name TEXT,
  status TEXT NOT NULL CHECK (status IN ('in_progress','success','error')),
  http_status INTEGER,
  error_category TEXT,
  started_at TEXT NOT NULL,
  completed_at TEXT,
  duration_ms INTEGER,
  ttft_ms INTEGER,
  upstream_ms INTEGER,
  input_tokens INTEGER,
  output_tokens INTEGER,
  total_tokens INTEGER
);
CREATE INDEX IF NOT EXISTS idx_telemetry_started ON telemetry_requests(started_at);
CREATE INDEX IF NOT EXISTS idx_telemetry_status ON telemetry_requests(status);
CREATE TABLE IF NOT EXISTS telemetry_attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id TEXT NOT NULL REFERENCES telemetry_requests(id) ON DELETE CASCADE,
  attempt_number INTEGER NOT NULL,
  provider_id INTEGER,
  provider_name TEXT NOT NULL,
  provider_prefix TEXT NOT NULL,
  provider_type TEXT NOT NULL,
  model_id INTEGER,
  upstream_model TEXT NOT NULL,
  credential_id INTEGER,
  started_at TEXT NOT NULL,
  completed_at TEXT,
  duration_ms INTEGER,
  http_status INTEGER,
  success INTEGER NOT NULL DEFAULT 0,
  error_category TEXT,
  input_tokens INTEGER,
  output_tokens INTEGER,
  total_tokens INTEGER
);
CREATE INDEX IF NOT EXISTS idx_attempt_request ON telemetry_attempts(request_id);
CREATE INDEX IF NOT EXISTS idx_attempt_provider ON telemetry_attempts(provider_id)`

var schema = []string{
	schemaProviders,
	schemaProviderKeys,
	schemaModels,
	schemaClients,
	schemaTelemetry,
	schemaGroups,
	schemaSettings,
}

const schemaSettings = `
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
)`

const schemaGroups = `
CREATE TABLE IF NOT EXISTS groups (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE,
  selection TEXT NOT NULL DEFAULT 'first' CHECK (selection IN ('first','round_robin')),
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS group_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  model_id INTEGER NOT NULL REFERENCES models(id) ON DELETE CASCADE,
  position INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (group_id, model_id)
);
CREATE INDEX IF NOT EXISTS idx_group_members_group ON group_members(group_id, position, id);
CREATE INDEX IF NOT EXISTS idx_group_members_model ON group_members(model_id)`
