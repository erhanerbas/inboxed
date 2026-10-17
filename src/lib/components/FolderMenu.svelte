<script lang="ts">
  import {
    Archive02Icon,
    ArrowDown01Icon,
    Delete02Icon,
    Folder01Icon,
    InboxIcon,
    NoteEditIcon,
    SentIcon,
    SpamIcon,
    StarIcon,
  } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { contextmenu } from "$lib/contextmenu";
  import { markAllReadMenu } from "$lib/menus";
  import { rise } from "$lib/motion";
  import { data, selectMailbox, ui } from "$lib/state.svelte";
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
  const current = $derived(boxes.find((m) => m.id === ui.mailboxKey));
  // Roles in Inboxed's order, whatever order the server lists them in; then folders as a tree, depth-first.
  const roleOrder = (m: Mailbox) => ROLES.findIndex((r) => r.role === m.role);
  const roles = $derived(boxes.filter((m) => m.role).toSorted((a, b) => roleOrder(a) - roleOrder(b)));
  const folders = $derived.by(() => {
    const out: { m: Mailbox; depth: number }[] = [];
    const walk = (parentId: string | null, depth: number) => {
      for (const m of boxes.filter((b) => !b.role && b.parentId === parentId)) {
        out.push({ m, depth });
        walk(m.id, depth + 1);
      }
    };
    walk(null, 0);
    return out;
  });

  let open = $state(false);
  let trigger = $state<HTMLButtonElement>();
  let panel = $state<HTMLElement>();

  function close(refocus = true) {
    open = false;
    if (refocus) trigger?.focus();
  }
  function pick(id: string) {
    selectMailbox(id);
    close();
  }
  // Opening puts focus on the selected folder, so Tab and Enter work from where you are.
  $effect(() => {
    if (open) panel?.querySelector<HTMLElement>("[aria-current]")?.focus();
  });
</script>

<svelte:window
  onclick={(e) => open && !panel?.contains(e.target as Node) && !trigger?.contains(e.target as Node) && close(false)}
  onkeydown={(e) => {
    if (open && e.key === "Escape") {
      e.preventDefault(); // handled here, so an open compose panel stays open
      close();
    }
  }}
/>

<div class="relative flex min-w-0 flex-1">
  <button
    bind:this={trigger}
    class="-ml-2 flex h-8 max-w-full items-center gap-1.5 rounded-lg px-2 text-heading transition-colors duration-100 hover:bg-ink/5 active:bg-ink/10"
    aria-haspopup="true"
    aria-expanded={open}
    aria-controls="folder-menu"
    onclick={() => (open = !open)}
  >
    <span class="truncate">{current?.name ?? "Folders"}</span>
    <Icon
      icon={ArrowDown01Icon}
      size={14}
      class="shrink-0 text-ink-muted transition-transform duration-150 motion-reduce:transition-none {open ? 'rotate-180' : ''}"
    />
  </button>

  {#if open}
    <nav
      bind:this={panel}
      id="folder-menu"
      aria-label="Folders"
      transition:rise
      class="shadow-float absolute top-full left-0 z-20 mt-1 max-h-[min(30rem,calc(100vh-6rem))] w-64 overflow-y-auto rounded-2xl border border-line bg-float p-1.5"
    >
      {#each roles as m (m.id)}
        {@render row(m, 0)}
      {/each}
      {#if folders.length}
        <div class="mx-2 my-1.5 border-t border-line"></div>
        {#each folders as { m, depth } (m.id)}
          {@render row(m, depth)}
        {/each}
      {/if}
    </nav>
  {/if}
</div>

{#snippet row(m: Mailbox, depth: number)}
  {@const selected = ui.mailboxKey === m.id}
  <button
    class="flex h-8 w-full items-center gap-2.5 rounded-lg pr-2 text-left transition-colors duration-100 {selected
      ? 'bg-selected font-medium'
      : 'hover:bg-ink/5 active:bg-ink/10'}"
    style:padding-left="{0.5 + depth * 1}rem"
    aria-current={selected ? "page" : undefined}
    onclick={() => pick(m.id)}
    use:contextmenu={() => markAllReadMenu(ui.accountId, m.id, m.unread)}
  >
    <Icon icon={m.role ? roleIcons[m.role] : Folder01Icon} size={16} class="shrink-0 {selected ? 'text-ink' : 'text-ink-muted'}" />
    <span class="min-w-0 flex-1 truncate">{m.name}</span>
    {#if m.unread}
      <!-- Inbox counts are accent pills (the accent means unread). -->
      <span
        class="text-caption tabular-nums {m.role === 'inbox'
          ? 'grid h-4.5 min-w-4.5 place-items-center rounded-full bg-accent px-1.5 font-semibold text-on-accent'
          : 'text-ink-muted'}"
      >
        {m.unread}
      </span>
    {/if}
  </button>
{/snippet}
