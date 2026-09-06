# pogu

pogu is a self-hosted router for AI model APIs. It sits between your
applications and your AI providers, giving you a single gateway with one set of
credentials, deterministic `prefix/model` routing, and a built-in admin
interface — all in one Go binary.

## Features (v1)

- **Single binary** — the admin web interface is embedded; no Node.js or
  separate frontend server at runtime
- **OpenAI-compatible API** — `POST /v1/chat/completions`, `GET /v1/models`
- **Anthropic-compatible API** — `POST /v1/messages`, `GET /v1/models`
- **Every model from either endpoint** — clients speak OpenAI or Anthropic;
  pogu translates only when a model's native upstream scheme differs,
  otherwise passing traffic through untouched
- **Streaming** — SSE with incremental delivery and cancellation propagation
- **Prefix routing** — models are addressed as `<provider-prefix>/<model-name>`
  (e.g. `oa/alpha`); routing is deterministic with no ambiguity
- **Client API keys** — `sk-pogu-...` credentials for your applications, shown
  once at creation, revocable at any time
- **Providers with Multiple API Keys** — configure multiple upstream API keys
  per provider with automatic failover on 401/403/429 before streaming begins
- **Encrypted credentials at rest** — provider API keys are sealed with
  AES-256-GCM under a master key that never leaves your machine
- **Local management CLI** — administer a running daemon over a Unix socket
  without the admin password
- **SQLite storage** — single-file persistence

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

## Quick start

```sh
./pogu init --data-dir ./data --password '<admin password, min 12 chars>'
./pogu serve --data-dir ./data
```

The admin interface is now at `http://127.0.0.1:8080/`. Log in with your admin
password, then:

1. **Add a provider** — name, native upstream API (`openai` or `anthropic`),
   a unique lowercase prefix, base URL, and API key. Use the **Test** button
   to verify connectivity.

2. **Add a model** to that provider, e.g. `alpha`. It is now addressable as
   `oa/alpha`.

3. **Create an API key** — the `sk-pogu-...` secret is shown once; copy it.

4. **Make requests**:

    OpenAI-compatible:

   ```sh
   curl http://127.0.0.1:8080/v1/chat/completions \
     -H "Authorization: Bearer $POGU_KEY" \
     -H 'Content-Type: application/json' \
     -d '{"model":"oa/alpha","messages":[{"role":"user","content":"hi"}]}'
   ```

   Anthropic-compatible (`x-api-key` or `Bearer` both work):

   ```sh
   curl http://127.0.0.1:8080/v1/messages \
     -H "x-api-key: $POGU_KEY" \
     -H 'anthropic-version: 2023-06-01' \
     -H 'Content-Type: application/json' \
     -d '{"model":"an/gamma","max_tokens":256,"messages":[{"role":"user","content":"hi"}]}'
   ```

   Add `"stream": true` for SSE streaming. `GET /v1/models` lists every enabled
   model across enabled providers as `prefix/model` IDs.

## CLI reference

```sh
pogu init --data-dir DIR --password PASSWORD   # first-time setup
pogu serve --data-dir DIR [--listen HOST:PORT] # start the daemon
pogu status --data-dir DIR                     # daemon + resource summary

  pogu provider list|create|get|update|delete|test --data-dir DIR [...]
pogu provider key list|create|get|update|enable|disable|primary|delete|test --data-dir DIR [...]
pogu model    list|create|get|update|delete    --data-dir DIR [...]
pogu key      list|create|revoke               --data-dir DIR [...]
pogu telemetry list [--limit N] --data-dir DIR  # recent request history
```

Management commands talk to a running daemon over a Unix socket inside the data
directory (mode 0600) and use the same application logic as the web API — no
admin password required, but only accessible to the local user.

`POGU_DATA_DIR` may be set instead of passing `--data-dir`; `POGU_ADMIN_PASSWORD`
can replace `--password` during init.

## Data layout

```text
data/
  config.json   runtime configuration
  pogu.db       SQLite database (providers, models, key hashes, sessions,
                request telemetry)
  master.key    256-bit master encryption key (0600)
  pogu.sock     daemon control socket (while serving)
```

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

## Scope notes (v1)

This is the MVP milestone. Deferred to the next milestone: logical model
groups, `round_robin` key/group selection, batch key import, the built-in
OpenCode Free provider with catalog sync, the `openai-responses` scheme,
and the Monitoring page with its live event stream. Telemetry is already
recorded for every request and inspectable via `pogu telemetry list`.

## License

TBD
