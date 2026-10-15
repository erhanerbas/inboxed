<script lang="ts">
  import ChevronLeft from "@lucide/svelte/icons/chevron-left";
  import Mail from "@lucide/svelte/icons/mail";
  import X from "@lucide/svelte/icons/x";
  import { rise } from "$lib/motion";
  import { ui } from "$lib/state.svelte";
  import type { Provider } from "$lib/types";

  // Connecting is wired up in Phase 3; for now every path just closes the dialog.
  const providers: { id: Provider; name: string; hint: string }[] = [
    { id: "gmail", name: "Gmail", hint: "Sign in with Google" },
    { id: "outlook", name: "Outlook", hint: "Sign in with Microsoft" },
    { id: "yahoo", name: "Yahoo", hint: "Email and app password" },
    { id: "imap", name: "Other", hint: "IMAP and SMTP" },
  ];

  let provider = $state<(typeof providers)[number]>();
  let dialog = $state<HTMLDialogElement>();
  $effect(() => dialog?.showModal());
</script>

<dialog
  bind:this={dialog}
  transition:rise
  onclose={() => (ui.addingAccount = false)}
  aria-labelledby="add-account-title"
  class="m-auto w-[28rem] max-w-[calc(100vw-2rem)] rounded-2xl border border-line bg-raised p-6 text-ink shadow-[0_24px_60px_rgb(0_0_0/0.5)] backdrop:bg-canvas/60"
>
  <header class="mb-5 flex items-center gap-2">
    {#if provider}
      <button class="icon-btn -ml-2" aria-label="Back" onclick={() => (provider = undefined)}>
        <ChevronLeft size={16} />
      </button>
    {/if}
    <h2 id="add-account-title" class="flex-1 text-heading">{provider?.name ?? "Add an account"}</h2>
    <button class="icon-btn -mr-2" aria-label="Close" onclick={() => dialog?.close()}><X size={16} /></button>
  </header>

  {#if !provider}
    <ul class="grid grid-cols-2 gap-2">
      {#each providers as p (p.id)}
        <li>
          <button
            class="flex w-full flex-col items-start rounded-xl border border-line p-4 text-left transition-colors duration-100 hover:bg-panel active:bg-panel"
            onclick={() => (provider = p)}
          >
            <Mail size={18} class="text-accent" />
            <span class="mt-3 font-semibold">{p.name}</span>
            <span class="text-caption text-ink-muted">{p.hint}</span>
          </button>
        </li>
      {/each}
    </ul>
  {:else if provider.id === "gmail" || provider.id === "outlook"}
    <p class="text-ink-muted">
      Inboxed opens your browser so you can sign in with {provider.name}. Inboxed never sees your password.
    </p>
    <form method="dialog"><button class="btn-primary mt-5 w-full">Continue in browser</button></form>
  {:else}
    <form method="dialog" class="flex flex-col gap-3">
      <label class="stack-field">Email<input type="email" required autocomplete="email" /></label>
      <label class="stack-field">
        {provider.id === "yahoo" ? "App password" : "Password"}<input type="password" required />
      </label>
      {#if provider.id === "imap"}
        {@render server("Incoming mail (IMAP)", "imap.example.com", 993)}
        {@render server("Outgoing mail (SMTP)", "smtp.example.com", 465)}
      {/if}
      <button class="btn-primary mt-2">Add account</button>
    </form>
  {/if}
</dialog>

{#snippet server(legend: string, host: string, port: number)}
  <fieldset class="grid grid-cols-[1fr_6rem] gap-3">
    <legend class="mb-2 text-caption font-medium text-ink-muted uppercase">{legend}</legend>
    <label class="stack-field">Server<input required placeholder={host} /></label>
    <label class="stack-field">Port<input type="number" required min="1" max="65535" value={port} /></label>
  </fieldset>
{/snippet}
