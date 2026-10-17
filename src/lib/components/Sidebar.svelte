<script lang="ts">
  import { Layers01Icon, Moon02Icon, PlusSignIcon, Sun01Icon } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import Avatar from "./Avatar.svelte";
  import { contextmenu } from "$lib/contextmenu";
  import { accountMenu, markAllReadMenu } from "$lib/menus";
  import { data, ui, selectAccount } from "$lib/state.svelte";
  import { theme, toggleTheme } from "$lib/theme.svelte";
  import { visibleMailboxes } from "$lib/views";

  // Inbox of a rail entry, in the accountId/key convention `ui` uses.
  const inboxKey = (accountId: string) =>
    accountId === "all" ? "inbox" : (data.mailboxes.find((m) => m.accountId === accountId && m.role === "inbox")?.id ?? "");
  const inboxUnread = (accountId: string) =>
    visibleMailboxes(data.mailboxes, accountId).find((m) => m.id === inboxKey(accountId))?.unread ?? 0;
</script>

<!-- The accounts rail is the whole sidebar: folders live in the message list's switcher. -->
<nav aria-label="Accounts" class="sidebar-material relative flex h-full w-20 shrink-0 flex-col items-center gap-2 pt-14 pb-3">
  <!-- Title-bar strip: room for the traffic lights. -->
  <div data-tauri-drag-region class="absolute inset-x-0 top-0 h-13"></div>
  <button
    class="rail-btn"
    aria-label="All inboxes"
    title="All inboxes"
    aria-current={ui.accountId === "all" ? "true" : undefined}
    onclick={() => selectAccount("all")}
    use:contextmenu={() => markAllReadMenu("all", "inbox", inboxUnread("all"))}
  >
    <Icon icon={Layers01Icon} size={18} />
  </button>
  {#each data.accounts as a (a.id)}
    {@const unread = inboxUnread(a.id)}
    <button
      class="rail-btn"
      aria-label="{a.name}, {a.email}{unread ? `, ${unread} unread` : ''}{a.sync?.state === 'importing' ? ', importing' : ''}{a.sync?.state === 'error' ? ", couldn't sync" : ''}"
      title={a.email}
      aria-current={ui.accountId === a.id ? "true" : undefined}
      onclick={() => selectAccount(a.id)}
      use:contextmenu={() => accountMenu(a.id, inboxKey(a.id), inboxUnread(a.id))}
    >
      <Avatar name={a.name} color={a.color} size={28} />
      {#if unread}
        <span
          aria-hidden="true"
          class="absolute -top-1.5 -right-1.5 grid h-4.5 min-w-4.5 place-items-center rounded-full bg-accent px-1 text-caption font-semibold text-on-accent tabular-nums ring-2 ring-sidebar"
        >
          {unread}
        </span>
      {/if}
    </button>
  {/each}
  <button
    class="rail-btn mt-auto"
    aria-label={theme.current === "dark" ? "Switch to light mode" : "Switch to dark mode"}
    title={theme.current === "dark" ? "Light mode" : "Dark mode"}
    onclick={toggleTheme}
  >
    <Icon icon={theme.current === "dark" ? Sun01Icon : Moon02Icon} size={18} />
  </button>
  <button class="rail-btn" aria-label="Add account" title="Add account" onclick={() => (ui.addingAccount = true)}>
    <Icon icon={PlusSignIcon} size={18} />
  </button>
</nav>
