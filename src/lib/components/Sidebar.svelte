<script lang="ts">
  import {
    Archive02Icon,
    ArrowRight01Icon,
    Delete02Icon,
    Folder01Icon,
    InboxIcon,
    Layers01Icon,
    NoteEditIcon,
    PlusSignIcon,
    SentIcon,
    SpamIcon,
    StarIcon,
  } from "@hugeicons/core-free-icons";
  import { SvelteSet } from "svelte/reactivity";
  import Icon from "./Icon.svelte";
  import Avatar from "./Avatar.svelte";
  import { contextmenu } from "$lib/contextmenu";
  import { accountMenu, markAllReadMenu } from "$lib/menus";
  import { data, retryImport, ui, selectAccount, selectMailbox } from "$lib/state.svelte";
  import { ROLES, visibleMailboxes } from "$lib/views";
  import type { Mailbox } from "$lib/types";

  const roleIcons = {
    inbox: InboxIcon,
    starred: StarIcon,
    drafts: NoteEditIcon,
    sent: SentIcon,
    archive: Archive02Icon,
    spam: SpamIcon,
    trash: Delete02Icon,
  };

  const boxes = $derived(visibleMailboxes(data.mailboxes, ui.accountId));
  const folders = (parentId: string | null) => boxes.filter((m) => !m.role && m.parentId === parentId);
  const account = $derived(data.accounts.find((a) => a.id === ui.accountId));

  // Inbox of a rail entry, in the accountId/key convention `ui` uses.
  const inboxKey = (accountId: string) =>
    accountId === "all" ? "inbox" : (data.mailboxes.find((m) => m.accountId === accountId && m.role === "inbox")?.id ?? "");
  const inboxUnread = (accountId: string) =>
    visibleMailboxes(data.mailboxes, accountId).find((m) => m.id === inboxKey(accountId))?.unread ?? 0;

  // Servers list folders in any order; the sidebar always shows roles in Inboxed's order.
  const roleOrder = (m: Mailbox) => ROLES.findIndex((r) => r.role === m.role);
  const importing = $derived(data.accounts.filter((a) => a.sync?.state === "importing"));

  const folded = new SvelteSet<string>();
  const toggleFold = (id: string) => (folded.has(id) ? folded.delete(id) : folded.add(id));
</script>

<aside
  class="sidebar-material relative flex h-full shrink-0 overflow-hidden transition-[width] duration-200 ease-out motion-reduce:transition-none {ui.sidebarCollapsed
    ? 'w-20'
    : 'w-68'}"
>
  <!-- Title-bar strip: room for the traffic lights and the toolbar buttons +page places over it. -->
  <div data-tauri-drag-region class="absolute inset-x-0 top-0 h-13"></div>

  <nav aria-label="Accounts" class="rail-material flex w-20 shrink-0 flex-col items-center gap-2 pt-14 pb-3">
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
        aria-label="{a.name}, {a.email}{unread ? `, ${unread} unread` : ''}{a.sync?.state === 'importing' ? ', importing' : ''}"
        title={a.email}
        aria-current={ui.accountId === a.id ? "true" : undefined}
        onclick={() => selectAccount(a.id)}
        use:contextmenu={() => accountMenu(a.id, inboxKey(a.id), inboxUnread(a.id))}
      >
        <Avatar name={a.name} color={a.color} size={28} />
        {#if unread}
          <span
            aria-hidden="true"
            class="absolute -top-1 -right-1 grid h-4 min-w-4 place-items-center rounded-full bg-accent px-1 text-caption font-semibold text-canvas tabular-nums ring-2 ring-canvas"
          >
            {unread}
          </span>
        {/if}
      </button>
    {/each}
    <button class="rail-btn mt-auto" aria-label="Add account" title="Add account" onclick={() => (ui.addingAccount = true)}>
      <Icon icon={PlusSignIcon} size={18} />
    </button>
  </nav>

  <nav
    id="mailbox-tree"
    aria-label="Mailboxes"
    inert={ui.sidebarCollapsed}
    class="w-48 shrink-0 overflow-y-auto px-2 pt-14 pb-3 transition-opacity duration-150 motion-reduce:transition-none {ui.sidebarCollapsed
      ? 'opacity-0'
      : ''}"
  >
    <div class="px-2 pb-3">
      <h2 class="truncate text-heading" title={account?.name ?? "All Inboxes"}>{account?.name ?? "All Inboxes"}</h2>
      {#if account?.sync?.state === "importing"}
        {@const { done, total } = account.sync}
        <!-- No live region: a counter that changes every second and a half would talk over everything. -->
        <p class="text-caption text-ink-muted">
          {total ? `Importing ${done.toLocaleString()} of ${total.toLocaleString()}` : "Looking for mail…"}
        </p>
        <progress class="import mt-1.5" max={total || undefined} value={total ? done : undefined} aria-label="Import progress"
        ></progress>
      {:else if account?.sync?.state === "error"}
        <p class="text-caption text-ink-muted wrap-anywhere">Import stopped: {account.sync.error}</p>
        <button
          class="mt-1 text-caption font-medium text-ink underline underline-offset-2"
          onclick={() => retryImport(account.id)}
        >
          Try again
        </button>
      {:else if !account && importing.length}
        <p class="text-caption text-ink-muted">Importing mail…</p>
      {:else}
        <p class="truncate text-caption text-ink-muted" title={account?.email}>
          {account?.email ?? `${data.accounts.length} ${data.accounts.length === 1 ? "account" : "accounts"}`}
        </p>
      {/if}
    </div>
    {#each boxes.filter((m) => m.role).toSorted((a, b) => roleOrder(a) - roleOrder(b)) as m (m.id)}
      {@render row(m)}
    {/each}
    {#if folders(null).length}
      <h3 class="mt-5 mb-1 px-2 text-caption font-medium text-ink-muted uppercase">Folders</h3>
      {#each folders(null) as m (m.id)}
        {@render row(m)}
      {/each}
    {/if}
  </nav>
</aside>

{#snippet row(m: Mailbox)}
  {@const glyph = m.role ? roleIcons[m.role] : Folder01Icon}
  {@const selected = ui.mailboxKey === m.id}
  {@const children = folders(m.id)}
  {@const open = !folded.has(m.id)}
  <div class="relative">
    <button
      class="flex h-7 w-full items-center gap-2 rounded-md px-2 text-left transition-colors duration-100 {children.length
        ? 'pr-8'
        : ''} {selected ? 'bg-raised font-medium' : 'hover:bg-raised/60 active:bg-raised'}"
      aria-current={selected ? "page" : undefined}
      onclick={() => selectMailbox(m.id)}
      use:contextmenu={() => markAllReadMenu(ui.accountId, m.id, m.unread)}
    >
      <Icon icon={glyph} size={16} class="shrink-0 {selected ? 'text-accent' : 'text-ink-muted'}" />
      <span class="min-w-0 flex-1 truncate">{m.name}</span>
      {#if m.unread}
        <span class="text-caption tabular-nums {m.role === 'inbox' ? 'font-semibold text-accent' : 'text-ink-muted'}">
          {m.unread}
        </span>
      {/if}
    </button>
    {#if children.length}
      <button
        class="absolute top-0 right-0 grid size-7 place-items-center rounded-md text-ink-muted transition-colors duration-100 hover:text-ink active:bg-raised"
        aria-label="{m.name} subfolders"
        aria-expanded={open}
        onclick={() => toggleFold(m.id)}
      >
        <Icon
          icon={ArrowRight01Icon}
          size={14}
          class="transition-transform duration-150 motion-reduce:transition-none {open ? 'rotate-90' : ''}"
        />
      </button>
    {/if}
  </div>
  {#if children.length && open}
    <div class="ml-[15px] border-l border-line pl-1.5">
      {#each children as child (child.id)}
        {@render row(child)}
      {/each}
    </div>
  {/if}
{/snippet}
