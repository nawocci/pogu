<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type CavemanLevelMeta } from '../api';
  import { toast } from '../toast.svelte';
  import { reveal } from '../motion.svelte';
  import { navigate } from '../router.svelte';
  import { setAuthenticated } from '../authMorph.svelte';
  import Busy from '../components/Busy.svelte';
  import Dropdown from '../components/Dropdown.svelte';

  let current = $state('');
  let next = $state('');
  let confirm = $state('');
  let busyPassword = $state(false);
  let passwordError = $state('');

  let prompt = $state('');
  let promptEnabled = $state(false);
  let promptLoaded = $state(false);
  let busyPrompt = $state(false);
  let promptError = $state('');

  let cavemanEnabled = $state(false);
  let cavemanLevel = $state('full');
  let cavemanLevels = $state<CavemanLevelMeta[]>([]);
  let cavemanSyncedAt = $state('');
  let cavemanLoaded = $state(false);
  let busyCaveman = $state(false);
  let busyCavemanSync = $state(false);
  let cavemanError = $state('');

  const MAX_PROMPT = 16384;

  onMount(() => {
    void loadPrompt();
    void loadCaveman();
  });

  async function loadPrompt() {
    try {
      const p = await api.globalPrompt();
      prompt = p.system_prompt;
      promptEnabled = p.enabled;
      promptLoaded = true;
    } catch (e) {
      promptError = e instanceof Error ? e.message : String(e);
    }
  }

  async function loadCaveman() {
    try {
      const c = await api.cavemanSettings();
      cavemanEnabled = c.enabled;
      cavemanLevel = c.level;
      cavemanLevels = c.levels;
      cavemanSyncedAt = c.last_synced_at;
      cavemanLoaded = true;
    } catch (e) {
      cavemanError = e instanceof Error ? e.message : String(e);
    }
  }

  async function changePassword(event: SubmitEvent) {
    event.preventDefault();
    passwordError = '';
    if (!current) {
      passwordError = 'Current password is required.';
      return;
    }
    if (next.length < 12) {
      passwordError = 'New password must be at least 12 characters.';
      return;
    }
    if (next !== confirm) {
      passwordError = 'New passwords do not match.';
      return;
    }
    busyPassword = true;
    try {
      await api.changePassword(current, next);
      toast('success', 'Password changed — all sessions signed out');
      navigate('/login', { replace: true });
      setAuthenticated(false);
    } catch (err) {
      passwordError = err instanceof Error ? err.message : String(err);
    } finally {
      busyPassword = false;
    }
  }

  async function savePrompt(event: SubmitEvent) {
    event.preventDefault();
    promptError = '';
    if (prompt.length > MAX_PROMPT) {
      promptError = `System prompt must be at most ${MAX_PROMPT} characters.`;
      return;
    }
    busyPrompt = true;
    try {
      const p = await api.updateGlobalPrompt({ system_prompt: prompt, enabled: promptEnabled });
      prompt = p.system_prompt;
      promptEnabled = p.enabled;
      toast('success', 'Global system prompt saved');
    } catch (err) {
      promptError = err instanceof Error ? err.message : String(err);
    } finally {
      busyPrompt = false;
    }
  }

  async function saveCaveman(event?: SubmitEvent) {
    if (event) event.preventDefault();
    cavemanError = '';
    busyCaveman = true;
    try {
      const c = await api.updateCavemanSettings({ enabled: cavemanEnabled, level: cavemanLevel });
      cavemanEnabled = c.enabled;
      cavemanLevel = c.level;
      cavemanLevels = c.levels;
      cavemanSyncedAt = c.last_synced_at;
      toast('success', 'Caveman settings saved');
    } catch (err) {
      cavemanError = err instanceof Error ? err.message : String(err);
    } finally {
      busyCaveman = false;
    }
  }

  async function syncCaveman() {
    cavemanError = '';
    busyCavemanSync = true;
    try {
      const c = await api.syncCavemanSkill();
      cavemanEnabled = c.enabled;
      cavemanLevel = c.level;
      cavemanLevels = c.levels;
      cavemanSyncedAt = c.last_synced_at;
      toast('success', 'Caveman rules updated from GitHub');
    } catch (err) {
      cavemanError = err instanceof Error ? err.message : String(err);
    } finally {
      busyCavemanSync = false;
    }
  }
</script>

<header use:reveal={{ kind: 'rise', i: 0 }} class="mb-8 flex items-end justify-between gap-5">
  <div>
    <p class="mb-4 font-mono text-[11px] tracking-[0.1em] text-accent-ink uppercase">Management</p>
    <h1 class="leading-none">Configurations</h1>
    <p class="mt-2.5 text-sm text-tertiary">Console access and router-wide request defaults.</p>
  </div>
</header>

<div class="grid items-start gap-x-6 gap-y-[42px] wide:grid-cols-2">
  <div class="flex flex-col gap-y-[42px]">
    <section use:reveal={{ kind: 'rise', i: 1 }} aria-labelledby="password-h">
      <div class="sec-head">
        <h2 id="password-h">Administrator password</h2>
      </div>
      <form novalidate onsubmit={changePassword}>
        <label class="field">
          <span>Current password</span>
          <input type="password" bind:value={current} autocomplete="current-password" />
        </label>
        <div class="grid grid-cols-2 gap-4 max-compact:grid-cols-1">
          <label class="field">
            <span>New password</span>
            <input type="password" bind:value={next} autocomplete="new-password" placeholder="At least 12 characters" />
          </label>
          <label class="field">
            <span>Confirm new password</span>
            <input type="password" bind:value={confirm} autocomplete="new-password" />
          </label>
        </div>
        <p class="hint">Changing the password signs out all sessions, including this one.</p>
        {#if passwordError}<p class="error-line enter-blip">{passwordError}</p>{/if}
        <div class="modal-actions mt-5">
          <button type="submit" class="btn btn-primary" disabled={busyPassword} aria-busy={busyPassword}>
            <Busy busy={busyPassword} text="Change password" wide="Change password" />
          </button>
        </div>
      </form>
    </section>

    <section use:reveal={{ kind: 'rise', i: 2 }} aria-labelledby="caveman-h">
      <div class="sec-head">
        <h2 id="caveman-h">Caveman mode</h2>
        <button
          type="button"
          class="btn btn-sm"
          onclick={syncCaveman}
          disabled={busyCavemanSync}
          aria-busy={busyCavemanSync}
        >
          <Busy busy={busyCavemanSync} text={busyCavemanSync ? 'Updating…' : 'Fetch latest rules'} wide="Fetch latest rules" />
        </button>
      </div>
      {#if !cavemanLoaded && !cavemanError}
        <p class="flex items-center gap-3 py-6 text-sm text-tertiary">Loading…</p>
      {:else}
        <form novalidate onsubmit={saveCaveman}>
          <label class="field">
            <span>Intensity level</span>
            <Dropdown
              value={cavemanLevel}
              onchange={(v) => (cavemanLevel = v)}
              options={cavemanLevels.map((l) => ({ value: l.id, label: l.label, hint: l.description }))}
            />
            <p class="hint">
              Compresses response output tokens while keeping technical substance. Overridable per request via <code class="font-mono text-xs">X-Caveman</code> header. Rules sync weekly from GitHub.
            </p>
          </label>
          <div class="mb-4 flex items-center justify-between gap-4">
            <label class="flex items-center gap-2 text-[13px] text-secondary">
              <input type="checkbox" bind:checked={cavemanEnabled} class="size-4" />
              Enabled
            </label>
            {#if cavemanSyncedAt}
              <span class="font-mono text-[11px] text-tertiary">
                Rules: {new Date(cavemanSyncedAt).toLocaleDateString()}
              </span>
            {/if}
          </div>
          {#if cavemanError}<p class="error-line enter-blip">{cavemanError}</p>{/if}
          <div class="modal-actions mt-5">
            <button type="submit" class="btn btn-primary" disabled={busyCaveman} aria-busy={busyCaveman}>
              <Busy busy={busyCaveman} text="Save caveman" wide="Save caveman" />
            </button>
          </div>
        </form>
      {/if}
    </section>
  </div>

  <section use:reveal={{ kind: 'rise', i: 1 }} aria-labelledby="prompt-h" class="flex flex-col h-full">
    <div class="sec-head">
      <h2 id="prompt-h">Global system prompt</h2>
    </div>
    {#if !promptLoaded && !promptError}
      <p class="flex items-center gap-3 py-6 text-sm text-tertiary">Loading…</p>
    {:else}
      <form novalidate onsubmit={savePrompt} class="flex flex-col flex-1">
        <label class="field flex flex-col flex-1">
          <span>Preferences applied to every request</span>
          <textarea
            bind:value={prompt}
            placeholder="e.g. Always answer concisely."
            class="min-h-[260px] flex-1 font-mono"
          ></textarea>
          <p class="hint">
            Appended after the request's own system instructions, wrapped in a
            <code class="font-mono text-xs">Pogu router preferences</code> marker so it stays
            identifiable. Applies to both endpoints and every model.
            Telemetry never stores prompt text.
          </p>
        </label>
        <div class="mb-4 flex items-center justify-between gap-4">
          <label class="flex items-center gap-2 text-[13px] text-secondary">
            <input type="checkbox" bind:checked={promptEnabled} class="size-4" />
            Enabled
          </label>
          <span class="font-mono text-[11px] text-tertiary">{prompt.length} / {MAX_PROMPT}</span>
        </div>
        {#if promptError}<p class="error-line enter-blip">{promptError}</p>{/if}
        <div class="modal-actions mt-auto pt-5">
          <button type="submit" class="btn btn-primary" disabled={busyPrompt} aria-busy={busyPrompt}>
            <Busy busy={busyPrompt} text="Save prompt" wide="Save prompt" />
          </button>
        </div>
      </form>
    {/if}
  </section>
</div>
