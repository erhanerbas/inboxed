<script lang="ts">
  import Archive from "@lucide/svelte/icons/archive";
  import FilePen from "@lucide/svelte/icons/file-pen";
  import Folder from "@lucide/svelte/icons/folder";
  import Inbox from "@lucide/svelte/icons/inbox";
  import Layers from "@lucide/svelte/icons/layers";
  import Plus from "@lucide/svelte/icons/plus";
  import Send from "@lucide/svelte/icons/send";
  import ShieldAlert from "@lucide/svelte/icons/shield-alert";
  import SquarePen from "@lucide/svelte/icons/square-pen";
  import Star from "@lucide/svelte/icons/star";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Avatar from "./Avatar.svelte";
  import { data, ui, selectAccount, selectMailbox } from "$lib/state.svelte";
  import { visibleMailboxes } from "$lib/views";
  import type { Mailbox } from "$lib/types";

  const roleIcons = {
    inbox: Inbox,
    starred: Star,
    drafts: FilePen,
    sent: Send,
    archive: Archive,
    spam: ShieldAlert,
    trash: Trash2,
  };

  const boxes = $derived(visibleMailboxes(data.mailboxes, ui.accountId));
  const folders = (parentId: string | null) => boxes.filter((m) => !m.role && m.parentId === parentId);
  const title = $derived(
    ui.accountId === "all" ? "All Inboxes" : (data.accounts.find((a) => a.id === ui.accountId)?.name ?? ""),
  );
</script>

<aside class="sidebar-material flex h-full w-64 shrink-0 flex-col">
  <div data-tauri-drag-region class="flex h-13 shrink-0 items-center justify-end px-2">
    <button class="icon-btn" aria-label="New message" title="New message (⌘N)" onclick={() => (ui.composing = true)}>
      <SquarePen size={17} />
    </button>
  </div>

  <div class="flex min-h-0 flex-1">
    <nav aria-label="Accounts" class="flex w-16 shrink-0 flex-col items-center gap-1.5 pb-3">
      <button
        class="rail-btn"
        aria-label="All inboxes"
        title="All inboxes"
        aria-pressed={ui.accountId === "all"}
        onclick={() => selectAccount("all")}
      >
        <Layers size={18} />
      </button>
      {#each data.accounts as a (a.id)}
        <button
          class="rail-btn"
          aria-label="{a.name}, {a.email}"
          title={a.email}
          aria-pressed={ui.accountId === a.id}
          onclick={() => selectAccount(a.id)}
        >
          <Avatar name={a.name} color={a.color} size={26} />
        </button>
      {/each}
      <button class="rail-btn mt-auto" aria-label="Add account" title="Add account" onclick={() => (ui.addingAccount = true)}>
        <Plus size={18} />
      </button>
    </nav>

    <nav aria-label="Mailboxes" class="min-w-0 flex-1 overflow-y-auto border-l border-line px-2 pb-3">
      <h2 class="truncate px-2 pb-2 text-heading">{title}</h2>
      {#each boxes.filter((m) => m.role) as m (m.id)}
        {@render row(m)}
      {/each}
      {#if folders(null).length}
        <h3 class="mt-5 mb-1 px-2 text-caption font-medium text-ink-muted uppercase">Folders</h3>
        {#each folders(null) as m (m.id)}
          {@render row(m)}
        {/each}
      {/if}
    </nav>
  </div>
</aside>

{#snippet row(m: Mailbox)}
  {@const Icon = m.role ? roleIcons[m.role] : Folder}
  {@const selected = ui.mailboxKey === m.id}
  <button
    class="flex h-7 w-full items-center gap-2 rounded-md px-2 text-left transition-colors duration-100 {selected
      ? 'bg-raised'
      : 'hover:bg-raised/60 active:bg-raised'}"
    aria-current={selected ? "page" : undefined}
    onclick={() => selectMailbox(m.id)}
  >
    <Icon size={16} class="shrink-0 {selected ? 'text-accent' : 'text-ink-muted'}" />
    <span class="min-w-0 flex-1 truncate">{m.name}</span>
    {#if m.unread}
      <span class="text-caption tabular-nums {m.role === 'inbox' ? 'font-semibold text-accent' : 'text-ink-muted'}">
        {m.unread}
      </span>
    {/if}
  </button>
  {#if folders(m.id).length}
    <div class="ml-[15px] border-l border-line pl-1.5">
      {#each folders(m.id) as child (child.id)}
        {@render row(child)}
      {/each}
    </div>
  {/if}
{/snippet}
