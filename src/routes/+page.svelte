<script lang="ts">
  import { api } from "$lib/engine";

  const health = api<{ status: string }>("/health");
</script>

<main class="grid h-screen place-items-center bg-white text-neutral-900 dark:bg-neutral-900 dark:text-neutral-100">
  <div class="text-center">
    <h1 class="text-2xl font-semibold">Inboxed</h1>
    <p class="mt-2 text-sm text-neutral-500">
      {#await health}
        Connecting to engine…
      {:then h}
        Engine: {h.status}
      {:catch e}
        <span class="text-red-600">Engine error: {e.message}</span>
      {/await}
    </p>
  </div>
</main>
