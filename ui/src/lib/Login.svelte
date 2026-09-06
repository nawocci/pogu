<script lang="ts">
  import { api } from './api';
  import { router, navigate, getSafeRedirect } from './router.svelte';
  import { setAuthenticated } from './authMorph.svelte';
  import { reveal } from './motion.svelte';
  import Busy from './components/Busy.svelte';

  let password = $state('');
  let busy = $state(false);
  let error = $state('');

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    error = '';
    busy = true;
    try {
      await api.login(password);
      password = '';
      navigate(getSafeRedirect(router.search, '/'), { replace: true });
      setAuthenticated(true);
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<main use:reveal={{ kind: 'rise', i: 0 }} class="w-full px-6 py-6" aria-labelledby="auth-h">
  <header class="mb-5 flex justify-center">
    <span class="brand-mark">POGU</span>
  </header>
  <h1 id="auth-h" class="text-lg leading-tight">Unlock console</h1>
  <p class="mb-5 mt-2 text-[13px] leading-relaxed text-tertiary">
    Enter your password to continue.
    <span class="text-muted">Sessions expire after 24 hours.</span>
  </p>

  <form novalidate onsubmit={submit} class="flex flex-col gap-3">
    <label class="field !mb-0">
      <span class="sr-only">Password</span>
      <input
        type="password"
        bind:value={password}
        autocomplete="current-password"
        placeholder="Password"
      />
    </label>

    {#if error}<p class="error-line enter-blip" role="alert">{error}</p>{/if}

    <button
      type="submit"
      class="btn btn-primary w-full justify-center"
      disabled={busy}
      aria-busy={busy}
    >
      <Busy busy={busy} text={busy ? 'Signing in…' : 'Unlock'} wide="Signing in…" />
    </button>
  </form>
</main>
