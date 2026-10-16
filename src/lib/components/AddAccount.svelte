<script lang="ts">
  import { Cancel01Icon } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { rise } from "$lib/motion";
  import { addAccount, ui } from "$lib/state.svelte";
  import type { AccountSettings, Server } from "$lib/types";

  // Gmail, Outlook and Yahoo sign-in arrive in Phase 8; until then every account is IMAP/SMTP.
  let dialog = $state<HTMLDialogElement>();
  $effect(() => dialog?.showModal());

  const settings = $state<AccountSettings>({
    name: "",
    email: "",
    username: "",
    password: "",
    imap: { host: "", port: 993, security: "tls" },
    smtp: { host: "", port: 465, security: "tls" },
  });
  let error = $state("");
  let busy = $state(false);

  // Most providers name their servers imap.<domain> and smtp.<domain>: a starting point the person can edit.
  function suggestServers() {
    const domain = settings.email.split("@")[1]?.trim();
    if (!domain) return;
    settings.imap.host ||= `imap.${domain}`;
    settings.smtp.host ||= `smtp.${domain}`;
  }

  // Each security mode has a standard port. Follow it, unless the port was changed by hand.
  const standardPorts = { imap: { tls: 993, starttls: 143 }, smtp: { tls: 465, starttls: 587 } };
  function setSecurity(side: "imap" | "smtp", security: Server["security"]) {
    const server = settings[side];
    if (server.port === standardPorts[side][server.security]) server.port = standardPorts[side][security];
    server.security = security;
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = "";
    const refused = await addAccount($state.snapshot(settings));
    busy = false;
    if (refused) error = refused;
    else dialog?.close();
  }
</script>

<dialog
  bind:this={dialog}
  transition:rise
  onclose={() => (ui.addingAccount = false)}
  aria-labelledby="add-account-title"
  class="m-auto max-h-[calc(100vh-2rem)] w-[28rem] max-w-[calc(100vw-2rem)] overflow-y-auto rounded-2xl border border-line bg-raised p-6 text-ink shadow-[0_24px_60px_rgb(0_0_0/0.5)] backdrop:bg-canvas/60"
>
  <header class="mb-5 flex items-center gap-2">
    <h2 id="add-account-title" class="flex-1 text-heading">Add an account</h2>
    <button class="icon-btn -mr-2" aria-label="Close" onclick={() => dialog?.close()}><Icon icon={Cancel01Icon} size={16} /></button>
  </header>

  <form class="flex flex-col gap-3" onsubmit={submit}>
    <label class="stack-field">Account name<input bind:value={settings.name} placeholder="Personal" autocomplete="off" /></label>
    <label class="stack-field">
      Email<input type="email" required bind:value={settings.email} onblur={suggestServers} autocomplete="email" />
    </label>
    <label class="stack-field">
      Password<input type="password" required bind:value={settings.password} autocomplete="current-password" />
    </label>
    <label class="stack-field">
      Username<input bind:value={settings.username} placeholder="Same as email" autocomplete="username" />
    </label>
    {@render server("imap", "Incoming mail (IMAP)")}
    {@render server("smtp", "Outgoing mail (SMTP)")}
    {#if error}
      <p role="alert" class="rounded-lg bg-panel px-3 py-2 wrap-anywhere">{error}</p>
    {/if}
    <button class="btn-primary mt-2" disabled={busy}>{busy ? "Signing in…" : "Add account"}</button>
  </form>
</dialog>

{#snippet server(side: "imap" | "smtp", legend: string)}
  <!-- Server, port and security on one row: the three always change together. -->
  <fieldset class="mt-2 grid grid-cols-[1fr_4.5rem_7rem] gap-3">
    <legend class="mb-2 text-caption font-medium text-ink-muted uppercase">{legend}</legend>
    <label class="stack-field">
      Server<input required bind:value={settings[side].host} placeholder="{side}.example.com" autocomplete="off" />
    </label>
    <label class="stack-field">Port<input type="number" required min="1" max="65535" bind:value={settings[side].port} /></label>
    <label class="stack-field">
      Security
      <select
        value={settings[side].security}
        onchange={(e) => setSecurity(side, e.currentTarget.value as Server["security"])}
      >
        <option value="tls">SSL/TLS</option>
        <option value="starttls">STARTTLS</option>
      </select>
    </label>
  </fieldset>
{/snippet}
