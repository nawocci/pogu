<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type ApiKey } from '../api';
  import { toast } from '../toast.svelte';
  import { reveal } from '../motion.svelte';
  import { createArmed } from '../armed.svelte';
  import { prettyDate } from '../format';
  import Lamp from '../components/Lamp.svelte';
  import KeyModal from './KeyModal.svelte';
  import SecretModal from './SecretModal.svelte';
  import Fit from '../components/Fit.svelte';

  let keys = $state<ApiKey[]>([]);
  let loaded = $state(false);
  let error = $state('');
  let showCreate = $state(false);
  let secret = $state('');
  const armed = createArmed();

  async function refresh() {
    try {
      keys = (await api.keys()) ?? [];
      loaded = true;
      error = '';
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }

  onMount(() => void refresh());

  async function revoke(key: ApiKey) {
    try {
      await api.revokeKey(key.id);
      await refresh();
      toast('success', `API key "${key.name}" revoked`);
    } catch (e) {
      toast('error', e instanceof Error ? e.message : String(e));
    }
  }

  const baseUrl = window.location.origin;
  let inlet = $state<'openai' | 'anthropic'>('openai');
  const curlExample =
    `curl ${baseUrl}/v1/chat/completions \\\n` +
    `  -H "Authorization: Bearer sk-pogu-…" \\\n` +
    `  -H "Content-Type: application/json" \\\n` +
    `  -d '{"model": "provider/model", "messages": [{"role": "user", "content": "Hello"}]}'`;
  const curlExampleAnthropic =
    `curl ${baseUrl}/v1/messages \\\n` +
    `  -H "x-api-key: sk-pogu-…" \\\n` +
    `  -H "anthropic-version: 2023-06-01" \\\n` +
    `  -H "Content-Type: application/json" \\\n` +
    `  -d '{"model": "provider/model", "max_tokens": 256, "messages": [{"role": "user", "content": "Hello"}]}'`;
</script>

<header use:reveal={{ kind: 'rise', i: 0 }} class="mb-8 flex flex-wrap items-end justify-between gap-5">
  <div>
    <p class="mb-4 font-mono text-[11px] tracking-[0.1em] text-accent-ink uppercase">Management</p>
    <h1 class="leading-none">Connections</h1>
    <p class="mt-2.5 text-sm text-tertiary">How clients reach pogu: API keys and the inference endpoint.</p>
  </div>
  <button use:reveal={{ kind: 'pop', i: 1 }} class="btn btn-primary" onclick={() => (showCreate = true)}>Create API key</button>
</header>

{#if error}
  <p use:reveal={{ kind: 'blip' }} class="error-line">{error}</p>
{/if}

<section use:reveal={{ kind: 'rise', i: 1 }} aria-labelledby="keys-h">
  <div class="sec-head">
    <h2 id="keys-h">Keys <span class="ml-[5px] font-mono text-[13px] text-accent-ink [vertical-align:3px]">{keys.length}</span></h2>
  </div>
  {#if !loaded}
    <div class="flex items-center gap-3 py-10 text-sm text-tertiary"><Lamp state="live" /> Loading keys…</div>
  {:else if keys.length === 0}
    <div use:reveal={{ kind: 'blip' }} class="empty">
      <h2 class="mb-2">No API keys yet</h2>
      <p class="mt-2 mb-[18px] max-w-[52ch] text-tertiary">
        Every inference request needs a bearer key. Create one per application so you can tell traffic apart
        and revoke access independently.
      </p>
      <button class="btn btn-primary" onclick={() => (showCreate = true)}>Create API key</button>
    </div>
  {:else}
    <table class="table-data table-cards">
      <thead>
        <tr><th>Name</th><th>Status</th><th>Created</th><th>Last used</th><th></th></tr>
      </thead>
      <tbody>
        {#each keys as k (k.id)}
          {@const armedNow = armed.is('k' + k.id)}
          <tr>
            <td data-label="Name" class="font-medium text-paper"><span class="block max-w-[220px] truncate">{k.name}</span></td>
            <td data-label="Status">
              {#if k.revoked_at}
                <Lamp state="err" label="Revoked" wide="Revoked" />
              {:else}
                <Lamp state="on" label="Active" wide="Active" />
              {/if}
            </td>
            <td data-label="Created"><time class="font-mono text-[11px] text-tertiary">{prettyDate(k.created_at)}</time></td>
            <td data-label="Last used"><time class="font-mono text-[11px] text-tertiary">{prettyDate(k.last_used_at)}</time></td>
            <td class="cell-actions text-right whitespace-nowrap">
              {#if !k.revoked_at}
                <button class="linkish linkish-del" onclick={() => armed.confirm('k' + k.id, () => revoke(k))}>
                  <Fit text={armedNow ? 'Confirm?' : 'Revoke'} wide="Confirm?" />
                </button>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    <p class="mt-3 text-xs text-tertiary">
      Revoked keys stop working permanently. The full secret is never shown again after creation.
    </p>
  {/if}
</section>

<section use:reveal={{ kind: 'rise', i: 2 }} aria-labelledby="guide-h" class="mt-[42px]">
  <div class="sec-head">
    <h2 id="guide-h">Quick guide</h2>
  </div>
  <p class="mb-4 max-w-[70ch] text-[13.5px] text-secondary">
    Pogu speaks both OpenAI Chat Completions and Anthropic Messages: every model —
    regardless of its native upstream API — is reachable from either endpoint,
    with translation applied only when the model's scheme differs. Point any compatible
    client at pogu: use
    <code class="font-mono text-xs text-accent-ink">{baseUrl}/v1</code>
    as the base URL and a generated key as the API key.
  </p>
  <div class="mb-4 inline-flex gap-0.5 rounded-sm border border-line bg-well p-0.5" role="group" aria-label="Client protocol">
    <button
      aria-pressed={inlet === 'openai'}
      class="rounded-[2px] px-3 py-1 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {inlet === 'openai'
        ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
        : 'text-tertiary hover:text-primary'}"
      onclick={() => (inlet = 'openai')}
    >OpenAI</button>
    <button
      aria-pressed={inlet === 'anthropic'}
      class="rounded-[2px] px-3 py-1 font-mono text-[10px] font-medium tracking-[0.07em] uppercase transition-colors {inlet === 'anthropic'
        ? 'bg-raised text-paper shadow-[0_0_0_1px_var(--line)]'
        : 'text-tertiary hover:text-primary'}"
      onclick={() => (inlet = 'anthropic')}
    >Anthropic</button>
  </div>
  <div class="grid gap-3 wide:grid-cols-2">
    <div class="panel overflow-hidden">
      <p class="border-b border-line-soft px-3.5 py-2 font-mono text-[10px] font-semibold tracking-[0.07em] uppercase text-tertiary">Client configuration</p>
      {#if inlet === 'openai'}
        <pre class="m-0 overflow-x-auto px-3.5 py-3 font-mono text-[12px] leading-relaxed text-primary">base_url = "{baseUrl}/v1"
api_key  = "sk-pogu-…"

# Authorization: Bearer sk-pogu-…</pre>
      {:else}
        <pre class="m-0 overflow-x-auto px-3.5 py-3 font-mono text-[12px] leading-relaxed text-primary">base_url = "{baseUrl}/v1"
api_key  = "sk-pogu-…"

# x-api-key: sk-pogu-…
# anthropic-version: 2023-06-01</pre>
      {/if}
    </div>
    <div class="panel overflow-hidden">
      <p class="border-b border-line-soft px-3.5 py-2 font-mono text-[10px] font-semibold tracking-[0.07em] uppercase text-tertiary">curl</p>
      <pre class="m-0 overflow-x-auto px-3.5 py-3 font-mono text-[12px] leading-relaxed text-primary">{inlet === 'openai' ? curlExample : curlExampleAnthropic}</pre>
    </div>
  </div>
</section>

{#if showCreate}
  <KeyModal
    onclose={() => (showCreate = false)}
    oncreated={(result) => {
      showCreate = false;
      secret = result.secret;
      void refresh();
    }}
  />
{/if}

{#if secret}
  <SecretModal {secret} onclose={() => (secret = '')} />
{/if}
