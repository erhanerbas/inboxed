<script lang="ts">
  import { onMount } from "svelte";
  import { Cancel01Icon, Delete02Icon } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { rise } from "$lib/motion";
  import { closeCompose, discardDraft, sendDraft, senders, ui } from "$lib/state.svelte";

  let form = $state<HTMLFormElement>();
  let to = $state<HTMLInputElement>();
  let body = $state<HTMLTextAreaElement>();
  let busy = $state<"" | "sending" | "saving">("");
  const from = $derived(senders());
  const isMac = navigator.platform.startsWith("Mac");

  // A reply already knows who it's for, so start in the message body, above the quote.
  onMount(() => {
    if (ui.draft.to && body) {
      body.focus();
      body.setSelectionRange(0, 0);
    } else to?.focus();
  });

  // Only the Send button and ⌘Enter send: the panel isn't a submitting form, so Enter in a field never does.
  async function send() {
    if (busy || !from.length) return;
    busy = "sending";
    await sendDraft();
    busy = "";
  }

  // Closing saves what was typed as a draft; if that fails the panel stays open, so nothing is lost.
  async function close() {
    if (busy) return;
    busy = "saving";
    await closeCompose();
    busy = "";
  }
</script>

<!-- Esc closes the panel unless something else already handled it (search, an IME, the add-account dialog).
     ⌘Enter (Ctrl+Enter elsewhere) sends while typing in it. -->
<svelte:window
  onkeydown={(e) => {
    if (e.key === "Escape" && !e.defaultPrevented && !e.isComposing && !ui.addingAccount && !ui.removingAccount) close();
    if (e.key === "Enter" && (isMac ? e.metaKey : e.ctrlKey) && form?.contains(document.activeElement)) {
      e.preventDefault();
      send();
    }
  }}
/>

<form
  bind:this={form}
  transition:rise
  aria-label="New message"
  onsubmit={(e) => e.preventDefault()}
  class="fixed right-6 bottom-6 z-20 flex h-[30rem] max-h-[calc(100vh-3rem)] w-[36rem] max-w-[calc(100vw-3rem)] flex-col overflow-hidden rounded-2xl border border-line bg-raised shadow-[0_24px_60px_rgb(0_0_0/0.5)]"
>
  <header class="flex h-11 shrink-0 items-center justify-between border-b border-line pr-2 pl-4">
    <h2 class="truncate font-semibold">{ui.draft.subject || "New message"}</h2>
    <button type="button" class="icon-btn" aria-label="Close and save draft" title="Close" onclick={close}>
      <Icon icon={Cancel01Icon} size={16} />
    </button>
  </header>

  <label class="field">
    <span>From</span>
    {#if from.length}
      <select bind:value={ui.draft.accountId}>
        {#each from as a (a.id)}
          <option value={a.id}>{a.name} · {a.email}</option>
        {/each}
      </select>
    {:else}
      <span class="w-auto! text-ink-muted">Add an account to send mail</span>
    {/if}
  </label>
  <!-- Plain text, not type=email: "Name <email>" is allowed, and the engine names any address it can't read. -->
  <label class="field">
    <span>To</span><input bind:this={to} bind:value={ui.draft.to} autocomplete="off" spellcheck="false" />
  </label>
  <label class="field"><span>Cc</span><input bind:value={ui.draft.cc} autocomplete="off" spellcheck="false" /></label>
  <label class="field"><span>Subject</span><input bind:value={ui.draft.subject} /></label>
  <textarea
    bind:this={body}
    bind:value={ui.draft.body}
    aria-label="Message"
    class="min-h-0 flex-1 resize-none bg-transparent px-4 py-3 text-read outline-none"
  ></textarea>

  {#if ui.composeError}
    <p role="alert" class="mx-3 mb-2 rounded-lg bg-panel px-3 py-2 wrap-anywhere">{ui.composeError}</p>
  {/if}
  <footer class="flex shrink-0 items-center justify-between border-t border-line px-3 py-2">
    <button type="button" class="icon-btn" aria-label="Discard draft" title="Discard draft" disabled={!!busy} onclick={discardDraft}>
      <Icon icon={Delete02Icon} size={16} />
    </button>
    <span class="flex items-center gap-3">
      {#if busy === "saving"}<span class="text-caption text-ink-muted">Saving draft…</span>{/if}
      <button type="button" class="btn-primary" disabled={!from.length || !!busy} title="Send ({isMac ? '⌘' : 'Ctrl+'}Enter)" onclick={send}>
        {busy === "sending" ? "Sending…" : "Send"}
      </button>
    </span>
  </footer>
</form>
