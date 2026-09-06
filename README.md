# pogu

pogu is a self-hosted router for AI model APIs. It sits between your
applications and your AI providers, giving you a single gateway with one set of
credentials, deterministic `prefix/model` routing, and a built-in admin
interface — all in one Go binary.

## Features

- **Single binary** — the admin web interface is embedded; build with
  `make build`, no Node.js needed at runtime
- **OpenAI-compatible API** — `POST /v1/chat/completions`, `GET /v1/models`
- **Anthropic-compatible API** — `POST /v1/messages`, `GET /v1/models`
- **Every model from either endpoint** — clients speak OpenAI or Anthropic;
  pogu translates only when a model's native upstream scheme differs,
  otherwise passing traffic through untouched
- **Streaming** — SSE with incremental delivery and cancellation propagation
- **Prefix routing** — models are addressed as `<provider-prefix>/<model-name>`
  (e.g. `or/mimo-v2.5-pro`); routing is deterministic with no ambiguity
- **Logical model groups** — client-facing logical model names that resolve to
  one or more configured targets, from static aliasing to multi-provider pools
- **Client API keys** — `sk-pogu-...` credentials for your applications, shown
  once at creation, revocable at any time
- **Providers with Multiple API Keys** — configure multiple upstream API keys
  per provider with configurable key selection and automatic failover
- **Key Selection Strategies** — choose between deterministic primary-first order
  (`first`) or independent per-provider round-robin (`round_robin`)
- **Safe Credential Failover** — automatically attempts the next eligible key when
  an upstream request fails with 401, 403, or 429 before streaming begins
- **Group target failover & round robin** — groups fail over across targets
  after key failover is exhausted, or rotate the starting target per request
  (`first` / `round_robin` selection strategies)
- **Batch Key Import** — import multiple upstream keys with line-oriented syntax
  (`name|key` or `key`), dry-run validation, duplicate rejection, and partial success
- **Encrypted credentials at rest** — provider API keys are sealed with
  AES-256-GCM under a master key that never leaves your machine; full secrets
  cannot be retrieved after creation
- **Local management CLI** — administer a running daemon over a Unix socket
  without the admin password
- **SQLite storage** — single-file persistence

Pogu speaks both OpenAI Chat Completions and Anthropic Messages: every
model is reachable from either endpoint, with translation applied only
when the model's scheme differs. Each model declares its
native upstream wire scheme (`openai`, `openai-responses`, or `anthropic`,
inheriting its provider's default unless pinned per model); requests to a
model whose scheme differs from the inlet are translated, otherwise passed
through untouched — including across group targets, which may freely mix
schemes with failover.

Translation is faithful but lossy at the edges: provider-opaque state
(Anthropic thinking signatures, Responses encrypted blobs) degrades to
reasoning text or is dropped, and features with no equivalent do not cross
over. Anything a stock client can express in chat semantics survives intact.

Pogu ships a permanent, disabled-by-default **OpenCode Free** provider
(`oc`) that needs no API key. Enable it and pogu syncs the live free-model
catalog into routable models automatically — a weekly background sync, or
`POST /api/providers/{id}/sync` (also `pogu provider sync`) to refresh on
demand. The provider cannot be deleted, only disabled; its
models are sync-managed. The free-model set and per-model wire schemes come
from the Zen docs page (cached under `data/`); if the docs cannot be fetched
the sync fails loudly and changes nothing.

## Requirements

- Go 1.27+ to build (Node.js 22+ only needed to modify the admin UI)

## Build

```sh
make build
```

This runs `npm ci && npm run build` in `ui/` to produce the embedded assets
(`internal/web/dist/`, git-ignored), then compiles the Go binary.

Other useful targets:

```sh
make test    # go test -race ./...
make vet     # go vet ./...
make check   # vet + test + TypeScript type check
make clean   # remove pogu binary and UI build output
```

## Docker

```sh
docker build -t pogu .
docker run -d --name pogu -p 127.0.0.1:8099:8099 -v pogu-data:/data --restart unless-stopped pogu
```

Or with compose:

```sh
docker compose up -d --build
```

Prebuilt images publish to `ghcr.io/nawocci/pogu` on pushes to
`master` (`:latest` plus short-SHA tags) and on version tags.

The image is distroless and runs as a non-root user; all state lives
in `/data` (SQLite database, `master.key`, config), so keep it on a
volume. On first boot with an empty volume the server starts in setup
mode and prints a one-time setup token to its logs:

```sh
docker logs pogu 2>&1 | grep 'setup token'
```

Open the admin UI, enter the token, and choose an admin password
(at least 12 characters). Whoever completes setup first claims the
instance, so do this immediately and do not expose an unclaimed
instance to a network you do not trust. To update, rebuild and
recreate the container — the volume preserves everything.

Back up `/data/pogu.db` together with `/data/master.key`: losing the
master key makes stored provider credentials unrecoverable. Avoid
network filesystems for the volume; SQLite file locking may misbehave
on them. For public deployments, put the container behind a
TLS-terminating reverse proxy.

## Quick start

```sh
./pogu init --data-dir ./data --password '<admin password, min 12 chars>'
./pogu serve --data-dir ./data
```

The admin interface is now at `http://127.0.0.1:8080/`. Log in with your admin
password, then:

1. **Add a provider** — name, native upstream API (`openai`,
   `openai-responses`, or `anthropic`), a unique
   lowercase prefix, base URL, and API key.

   Use the **Test** button to verify connectivity (auth failures, bad URLs, and
   unreachable hosts are reported distinctly).

2. **Add a model** to that provider, e.g. `mimo-v2.5-pro`. It is now
   addressable as `or/mimo-v2.5-pro`.

3. **Create an API key** — the `sk-pogu-...` secret is shown once; copy it.

4. **Optionally create a group** under *Model groups* — e.g. add
   `or/mimo-v2.5-pro` (and any fallback targets) to a group named `frontier`.
   Clients can then call `"model": "frontier"` and pogu picks the first
   available target, failing over to the next one when a provider returns
   401, 403, or 429. The routing rule is simple: a model identifier
   containing `/` is a concrete `prefix/model` reference; without a slash it
   is a group name.

5. **Make requests**:

    OpenAI-compatible:

   ```sh
   curl http://127.0.0.1:8080/v1/chat/completions \
     -H "Authorization: Bearer $POGU_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"or/mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}'
   ```

   Anthropic-compatible (`x-api-key` or `Bearer` both work):

   ```sh
   curl http://127.0.0.1:8080/v1/messages \
     -H "x-api-key: $POGU_KEY" \
     -H 'anthropic-version: 2023-06-01' \
     -H 'Content-Type: application/json' \
     -d '{"model":"an/my-model","max_tokens":256,"messages":[{"role":"user","content":"hi"}]}'
   ```

   Add `"stream": true` for SSE streaming. `GET /v1/models` lists every enabled
   model across enabled providers as `prefix/model` IDs, plus enabled
   non-empty groups by name.

## Provider API Keys and Failover

Each provider can hold multiple upstream credentials. Manage them from the
provider detail page in the web UI or via the CLI:

### Key Selection Policy

- **Primary first (`first`, default)**: Requests attempt keys in deterministic order
  (primary key first).
- **Round robin (`round_robin`)**: Outbound requests rotate evenly across all enabled
  keys under the provider. Round-robin state is maintained independently per provider.

### Credential Failover

When an upstream returns a credential- or rate-limiting error (`401`, `403`, or `429`):
- pogu automatically tries the next eligible key in the provider pool without repeating
  previously attempted keys for that request.
- Client requests succeed transparently if a subsequent key completes the request.
- Ordinary client errors (e.g. `400`), unrouted models, network dial failures, and `5xx`
  server errors do not trigger credential failover.
- Streaming responses will fail over only **before** the downstream response stream has
  started; once chunks are flushed to the client, failover is aborted to avoid duplicating
  output.

### Batch Import Syntax

Upload multiple keys at once with line-oriented entries:
```text
work|sk-work-key-1234
personal|sk-personal-5678
sk-unnamed-auto-assigned-key
```
Batch import validates each entry against existing keys and within the batch, rejecting
duplicate secrets and malformed entries while allowing valid entries to be imported.

## CLI reference

```sh
pogu init --data-dir DIR --password PASSWORD   # first-time setup
pogu serve --data-dir DIR [--listen HOST:PORT] # start the daemon
pogu status --data-dir DIR                     # daemon + resource summary

  pogu provider list|create|get|update|delete|test|sync --data-dir DIR [...]
pogu provider key list|create|get|update|enable|disable|primary|delete|test|import --data-dir DIR [...]
pogu model    list|create|get|update|delete    --data-dir DIR [...]
pogu key      list|create|revoke               --data-dir DIR [...]
pogu telemetry list [--limit N] --data-dir DIR  # recent request history
```

Management commands talk to a running daemon over a Unix socket inside the data
directory (mode 0600) and use the same application logic as the web API — no
admin password required, but only accessible to the local user.

`POGU_DATA_DIR` may be set instead of passing `--data-dir`; `POGU_ADMIN_PASSWORD`
can replace `--password` during init. `POGU_OPENCODE_DOCS_URL` overrides the
Zen docs source used for free-model discovery (defaults to the upstream docs
page).

## Data layout

```text
data/
  config.json   runtime configuration
  pogu.db       SQLite database (providers, models, key hashes, sessions,
                request telemetry)
  master.key    256-bit master encryption key (0600)
  opencode-docs-cache.json  cached Zen docs free-model set (refreshed by sync)
  pogu.sock     daemon control socket (while serving)
```

## API

### AI-facing (client API key required)

| Method | Path                   | Description                        |
|--------|------------------------|------------------------------------|
| POST   | `/v1/chat/completions` | Canonical OpenAI-compatible chat (streamable)  |
| POST   | `/v1/messages`         | Anthropic inlet, translated to canonical (streamable) |
| GET    | `/v1/models`           | List enabled models and groups     |

### Operational

| Method | Path      | Description              |
|--------|-----------|--------------------------|
| GET    | `/healthz`| Liveness probe           |

### Management (`/api/...`, admin session via login)

Full CRUD for providers, models, model groups (including member management
and ordering), and API keys, plus provider connectivity testing, catalog sync
for the built-in provider, and live/historical monitoring queries. The
Configurations page covers the rest: administrator password changes (`POST
/api/auth/password`, current password required, all sessions rotate) and the
global system prompt (`GET`/`PUT /api/settings/prompt`). Provider
credentials are never returned after storage; key secrets are returned only
at creation.

### Global system prompt

Persistent user-level preferences, like a router-side `AGENTS.md`: when
enabled, the prompt is appended after the request's own system instructions
under a `Pogu router preferences` marker (a new first message when the
request has no system content), on both endpoints and for every model.
Request content is never modified beyond this append, and telemetry never
stores prompt text.

## Request telemetry

Every AI request proxied by pogu is recorded as operational metadata in two
SQLite tables: one row per request and one row per upstream attempt. Recorded
per request: a random request ID (also returned as `X-Request-Id`), inbound
protocol, the public model identifier as the client sent it, streaming flag,
client key ID, outcome, HTTP status, error category, timing, and token usage
when the upstream reports it. Each attempt records the provider (snapshot of
id, name, prefix, type), upstream model, result, and usage.

Telemetry is metadata only — prompts, completions, request/response bodies,
and secrets are never stored. Requests that fail before reaching a provider
(e.g. unknown model) are not recorded. Telemetry failures never affect
request handling. Inspect records with `pogu telemetry list` or the
Monitoring page in the admin console (live activity, usage charts, and
filterable request history).

## Logical model groups

Groups decouple the model name your clients use from the provider-specific
identifier upstream. Two shapes share one abstraction:

- **Stable alias** (one member): the same logical model exists under
  different upstream names.
- **Quota/fallback pool** (multiple members): clients call `frontier` and
  pogu tries each target in order, moving on when the upstream returns 401,
  403, or 429 before the response starts. Within one target, all of the
  provider's eligible API keys are tried before the next target.

Each group has a **selection strategy**:

- **First** (default) — every request starts at the first available target
  and fails over down the configured order.
- **Round robin** — the starting target rotates per request across the
  available targets; failover then follows the configured order, wrapping
  the ring.

Members whose provider/model is disabled, or whose provider has no enabled
credentials, are skipped at resolution time. If no target is available the
request fails fast with `503 group has no available targets`.

## Security

- Provider API keys are encrypted at rest (AES-256-GCM) with `master.key`.
  **Losing `master.key` makes stored provider credentials unrecoverable** —
  back it up somewhere safe.
- The admin password is stored only as an Argon2id hash. Sessions are random
  tokens stored hashed, expire after 24 hours, and are all invalidated when the
  password changes. Login attempts are throttled per source.
- Client API keys are stored only as SHA-256 hashes.
- The database, master key, and data directory are created with restrictive
  permissions (0600/0700). Upstream redirects are never followed, so provider
  credentials cannot leak to a redirect target.
- The daemon binds to 127.0.0.1 by default. For public deployments, put it
  behind a TLS-terminating reverse proxy.

## Testing

```sh
go test ./...
```

A full-system integration harness (deterministic mock OpenAI/Anthropic providers,
no paid accounts needed) lives in the bringup workspace next to this repo:
`pogu-bringup/scripts/bringup`.
