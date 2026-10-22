<script lang="ts">
  import Avatar from "./Avatar.svelte";
  import { data, renameAccount, senders, ui, updateBadge } from "$lib/state.svelte";
  import { FONTS, saveSettings, settings } from "$lib/settings.svelte";
  import { chooseTheme, theme } from "$lib/theme.svelte";

  let dialog = $state<HTMLDialogElement>();
  $effect(() => dialog?.showModal());

  const tabs = ["General", "Reading", "Notifications", "Accounts"] as const;
  let tab = $state<(typeof tabs)[number]>("General");
  const themes = [
    { id: "system", name: "System" },
    { id: "light", name: "Light" },
    { id: "dark", name: "Dark" },
  ] as const;

  // Tabs as the platform does them: arrows move between them, and the tab follows focus.
  function arrows(e: KeyboardEvent) {
    const step = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    tab = tabs[(tabs.indexOf(tab) + step + tabs.length) % tabs.length];
    document.getElementById(`settings-tab-${tab}`)?.focus();
  }
</script>

<!-- Choices apply as they're made: there's nothing to save, so the only button is Done. -->
<dialog
  bind:this={dialog}
  onclose={() => (ui.settingsOpen = false)}
  aria-labelledby="settings-title"
  class="m-auto flex h-[min(36rem,calc(100vh-4rem))] w-[34rem] max-w-[calc(100vw-2rem)] flex-col shadow-float rounded-2xl border border-line bg-float text-ink backdrop:bg-ink/30"
>
  <header class="px-6 pt-5">
    <h2 id="settings-title" class="text-heading">Settings</h2>
    <div role="tablist" aria-label="Settings sections" tabindex="-1" class="mt-4 flex gap-1 border-b border-line" onkeydown={arrows}>
      {#each tabs as t (t)}
        <button
          id="settings-tab-{t}"
          role="tab"
          aria-selected={tab === t}
          aria-controls="settings-panel"
          tabindex={tab === t ? 0 : -1}
          class="-mb-px border-b-2 px-2.5 pb-2 transition-colors duration-100 active:text-ink {tab === t
            ? 'border-accent font-medium text-ink'
            : 'border-transparent text-ink-muted hover:text-ink'}"
          onclick={() => (tab = t)}
        >
          {t}
        </button>
      {/each}
    </div>
  </header>

  <div id="settings-panel" role="tabpanel" aria-labelledby="settings-tab-{tab}" class="min-h-0 flex-1 overflow-y-auto px-6 py-5">
    {#if tab === "General"}
      <fieldset>
        <legend class="font-medium">Appearance</legend>
        <div class="mt-2 inline-flex rounded-lg border border-line p-0.5">
          {#each themes as t (t.id)}
            <label
              class="rounded-md px-3 py-1 transition-colors duration-100 hover:bg-ink/5 active:bg-ink/10 has-checked:bg-selected has-checked:font-medium has-focus-visible:outline-2 has-focus-visible:outline-accent"
            >
              <input
                type="radio"
                name="theme"
                class="sr-only"
                checked={theme.choice === t.id}
                onchange={() => chooseTheme(t.id)}
              />
              {t.name}
            </label>
          {/each}
        </div>
        <p class="mt-1.5 text-caption text-ink-muted">System follows your Mac's light or dark appearance.</p>
      </fieldset>

      <fieldset class="mt-6">
        <legend class="font-medium">Mail font</legend>
        <p class="mt-1 text-caption text-ink-muted">Every message is shown in this font, whatever the sender chose.</p>
        <div class="mt-3 flex flex-col gap-1">
          {#each FONTS as f (f.id)}
            <label
              class="flex items-center gap-3 rounded-lg px-3 py-2 transition-colors duration-100 hover:bg-ink/5 active:bg-ink/10 has-checked:bg-selected"
            >
              <input
                type="radio"
                name="mail-font"
                checked={settings.mailFont === f.id}
                onchange={() => {
                  settings.mailFont = f.id;
                  saveSettings();
                }}
                class="accent-accent"
              />
              <span class="min-w-0 flex-1" style:font-family={f.family}>
                <span class="block text-read">{f.name}</span>
                <span class="block truncate text-ink-muted">Thanks for the notes. I'll send the draft on Monday.</span>
              </span>
            </label>
          {/each}
        </div>
      </fieldset>
    {:else if tab === "Reading"}
      {@render toggle("markRead", "Mark as read when opened", "Off: conversations stay unread until you mark them read.")}
      {@render toggle(
        "loadImages",
        "Load remote images automatically",
        "Senders can then tell when you open their mail, and where you are. Off: each message asks first.",
      )}
    {:else if tab === "Notifications"}
      {@render toggle(
        "notify",
        "Notify me about new mail",
        "One for each message, while Inboxed isn't the app in front or its window is closed. In front, a banner shows it.",
      )}
      <label class="stack-field py-3 {settings.notify ? '' : 'opacity-50'}">
        <span>Only from these domains</span>
        <textarea
          rows="3"
          disabled={!settings.notify}
          placeholder="northwind.studio"
          value={settings.notifyDomains}
          oninput={(e) => {
            settings.notifyDomains = e.currentTarget.value;
            saveSettings();
          }}
        ></textarea>
        <span class="font-normal">One per line; subdomains count too. Empty: mail from anyone.</span>
      </label>
      {@render toggle("sound", "Play a sound", "With each notification.", !settings.notify)}
      {@render toggle("dockBadge", "Unread count on the Dock icon", "The number of unread conversations in your inboxes.", false, updateBadge)}
    {:else}
      {#if !data.accounts.length}
        <p class="text-ink-muted">No accounts yet.</p>
      {/if}
      {#if data.accounts.length}
        <p class="mb-4 text-caption text-ink-muted">Click a name to change it. It's how the account shows in Inboxed and the sender name on mail you send from it.</p>
      {/if}
      <ul class="flex flex-col divide-y divide-line">
        {#each data.accounts as a (a.id)}
          <li class="py-4 first:pt-0">
            <div class="flex items-center gap-3">
              <Avatar name={a.name} color={a.color} size={28} />
              <span class="min-w-0 flex-1">
                <!-- Edited in place: Enter or leaving the field saves, Esc puts the name back (and keeps Settings open). -->
                <input
                  class="-mx-1.5 block h-7 w-[calc(100%+0.75rem)] min-w-0 rounded-md bg-transparent px-1.5 font-medium text-ink outline-none transition-colors duration-100 hover:bg-ink/5 focus:bg-canvas focus:ring-1 focus:ring-ink"
                  aria-label="Name of {a.email}"
                  title="Rename"
                  value={a.name}
                  spellcheck="false"
                  autocomplete="off"
                  onkeydown={(e) => {
                    if (e.key === "Enter") e.currentTarget.blur();
                    else if (e.key === "Escape") {
                      e.preventDefault();
                      e.currentTarget.value = a.name;
                      e.currentTarget.blur();
                    }
                  }}
                  onblur={(e) => {
                    renameAccount(a.id, e.currentTarget.value);
                    e.currentTarget.value = a.name; // trimmed, or the address when left empty
                  }}
                />
                <span class="block truncate text-caption text-ink-muted">{a.email}</span>
              </span>
              <button class="btn-secondary h-7 px-3 text-caption" onclick={() => (ui.removingAccount = a.id)}>Remove…</button>
            </div>
            {#if senders().some((s) => s.id === a.id)}
              <label class="stack-field mt-3">
                <span>Signature</span>
                <textarea
                  rows="3"
                  placeholder="Added to new messages, replies and forwards from this account"
                  value={settings.signatures[a.id] ?? ""}
                  oninput={(e) => {
                    settings.signatures[a.id] = e.currentTarget.value;
                    saveSettings();
                  }}
                ></textarea>
              </label>
            {/if}
          </li>
        {/each}
      </ul>
      <button class="btn-secondary mt-2" onclick={() => (ui.addingAccount = true)}>Add account</button>
    {/if}
  </div>

  <footer class="flex justify-end border-t border-line px-6 py-4">
    <button class="btn-primary" onclick={() => dialog?.close()}>Done</button>
  </footer>
</dialog>

{#snippet toggle(
  key: "markRead" | "loadImages" | "notify" | "sound" | "dockBadge",
  label: string,
  hint: string,
  disabled = false,
  after?: () => void,
)}
  <label class="flex items-start gap-4 py-3 first:pt-0 {disabled ? 'opacity-50' : ''}">
    <span class="min-w-0 flex-1">
      <span class="block font-medium">{label}</span>
      <span class="mt-0.5 block text-caption text-ink-muted">{hint}</span>
    </span>
    <input
      type="checkbox"
      role="switch"
      class="switch mt-0.5"
      {disabled}
      checked={settings[key]}
      onchange={(e) => {
        settings[key] = e.currentTarget.checked;
        saveSettings();
        after?.();
      }}
    />
  </label>
{/snippet}
