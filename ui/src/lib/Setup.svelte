<script lang="ts">
  import { api } from './api';
  import { setAuthenticated } from './authMorph.svelte';
  import { reveal } from './motion.svelte';
  import Busy from './components/Busy.svelte';

  let { ondone }: { ondone: () => void } = $props();

  let token = $state('');
  let password = $state('');
  let confirm = $state('');
  let busy = $state(false);
  let error = $state('');

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    if (!token.trim()) {
      error = 'Setup token is required — find it in the server logs.';
      return;
    }
    if (password.length < 12) {
      password = '';
      confirm = '';
      error = 'Password must be at least 12 characters.';
      return;
    }
    if (password !== confirm) {
      password = '';
      confirm = '';
      error = 'Passwords do not match.';
      return;
    }
    busy = true;
    try {
      await api.completeSetup(token.trim(), password);
      password = '';
      confirm = '';
      token = '';
      setAuthenticated(true);
      ondone();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<main use:reveal={{ kind: 'rise', i: 0 }} class="w-full px-6 py-6" aria-labelledby="setup-h">
  <header class="mb-5 flex justify-center">
    <span class="brand-mark">POGU</span>
  </header>
  <h1 id="setup-h" class="text-lg leading-tight">Set up pogu</h1>
  <p class="mb-5 mt-2 text-[13px] leading-relaxed text-tertiary">
    This instance has no administrator yet. Enter the one-time setup token
    from the server logs, then choose your admin password.
  </p>

  <form novalidate onsubmit={submit} class="flex flex-col gap-3">
    <label class="field !mb-0">
      <span>Setup token</span>
      <input
        type="text"
        bind:value={token}
        autocomplete="off"
        spellcheck={false}
        placeholder="Setup token from the server logs"
      />
    </label>
    <label class="field !mb-0">
      <span>Admin password</span>
      <input
        type="password"
        bind:value={password}
        autocomplete="new-password"
        placeholder="At least 12 characters"
      />
    </label>
    <label class="field !mb-0">
      <span>Confirm password</span>
      <input
        type="password"
        bind:value={confirm}
        autocomplete="new-password"
        placeholder="Repeat the password"
      />
    </label>

    {#if error}<p class="error-line enter-blip" role="alert">{error}</p>{/if}

    <button
      type="submit"
      class="btn btn-primary w-full justify-center"
      disabled={busy}
      aria-busy={busy}
    >
      <Busy busy={busy} text={busy ? 'Setting up…' : 'Claim instance'} wide="Setting up…" />
    </button>
  </form>
</main>
