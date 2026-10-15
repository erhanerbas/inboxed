<script lang="ts">
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import X from "@lucide/svelte/icons/x";
  import { rise } from "$lib/motion";
  import { data, ui } from "$lib/state.svelte";

  let to = $state<HTMLInputElement>();
  $effect(() => to?.focus());

  const close = () => (ui.composing = false);
  // Sending is wired up in Phase 5; for now Send just closes the panel.
  function send(e: SubmitEvent) {
    e.preventDefault();
    close();
  }
</script>

<!-- The add-account dialog handles its own Esc. -->
<svelte:window onkeydown={(e) => e.key === "Escape" && !ui.addingAccount && close()} />

<form
  transition:rise
  aria-label="New message"
  onsubmit={send}
  class="fixed right-6 bottom-6 z-20 flex h-[30rem] max-h-[calc(100vh-3rem)] w-[36rem] max-w-[calc(100vw-3rem)] flex-col overflow-hidden rounded-2xl border border-line bg-raised shadow-[0_24px_60px_rgb(0_0_0/0.5)]"
>
  <header class="flex h-11 shrink-0 items-center justify-between border-b border-line pr-2 pl-4">
    <h2 class="font-semibold">New message</h2>
    <button type="button" class="icon-btn" aria-label="Close" title="Close" onclick={close}><X size={16} /></button>
  </header>

  <label class="field">
    <span>From</span>
    <select value={ui.accountId === "all" ? data.accounts[0]?.id : ui.accountId}>
      {#each data.accounts as a (a.id)}
        <option value={a.id}>{a.name} · {a.email}</option>
      {/each}
    </select>
  </label>
  <label class="field"><span>To</span><input bind:this={to} type="email" multiple required /></label>
  <label class="field"><span>Cc</span><input type="email" multiple /></label>
  <label class="field"><span>Subject</span><input /></label>
  <textarea aria-label="Message" class="min-h-0 flex-1 resize-none bg-transparent px-4 py-3 text-read outline-none"></textarea>

  <footer class="flex shrink-0 items-center justify-between border-t border-line px-3 py-2">
    <button type="button" class="icon-btn" aria-label="Discard draft" title="Discard draft" onclick={close}>
      <Trash2 size={16} />
    </button>
    <button class="btn-primary">Send</button>
  </footer>
</form>
