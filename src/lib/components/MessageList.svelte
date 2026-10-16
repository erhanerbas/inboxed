<script lang="ts">
  import { Attachment01Icon, InboxIcon, Search01Icon, StarIcon } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { contextmenu } from "$lib/contextmenu";
  import { threadMenu } from "$lib/menus";
  import { data, ui, openThread } from "$lib/state.svelte";
  import { visibleMailboxes } from "$lib/views";
  import { formatDate } from "$lib/format";

  const list = $derived(data.list); // already filtered and searched by the engine
  const title = $derived(
    visibleMailboxes(data.mailboxes, ui.accountId).find((m) => m.id === ui.mailboxKey)?.name ?? "",
  );
  const accountColor = (id: string) => data.accounts.find((a) => a.id === id)?.color;

  // On the window, because WebKit doesn't focus a button when it's clicked.
  function move(e: KeyboardEvent) {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    // Only when the list, or nothing, has focus: arrows inside a field belong to that field.
    const target = e.target as HTMLElement;
    if (target !== document.body && !target.closest("#message-list")) return;
    e.preventDefault();
    const i = list.findIndex((t) => t.id === ui.threadId);
    const next = list[i === -1 ? 0 : i + (e.key === "ArrowDown" ? 1 : -1)];
    if (!next) return;
    openThread(next.id);
    document.getElementById(`thread-${next.id}`)?.focus();
  }
</script>

<svelte:window onkeydown={move} />

<section aria-label="Messages" class="flex h-full flex-col">
  <!-- With the sidebar collapsed, the toolbar buttons sit over this header's leading edge, so the title moves aside. -->
  <header
    data-tauri-drag-region
    class="flex h-13 shrink-0 items-center justify-between gap-3 pr-4 transition-[padding] duration-200 ease-out motion-reduce:transition-none {ui.sidebarCollapsed
      ? 'pl-[5.5rem]'
      : 'pl-4'}"
  >
    <h2 class="truncate text-heading">{title}</h2>
    <span class="shrink-0 text-caption text-ink-muted">
      {list.length}
      {list.length === 1 ? "conversation" : "conversations"}
    </span>
  </header>

  <div class="px-3 pb-2">
    <label class="flex h-8 items-center gap-2 rounded-lg bg-raised px-2.5 text-ink-muted focus-within:ring-2 focus-within:ring-accent">
      <Icon icon={Search01Icon} size={14} />
      <input
        bind:value={ui.query}
        placeholder="Search"
        aria-label="Search messages"
        onkeydown={(e) => {
          if (e.key !== "Escape") return;
          e.preventDefault(); // handled here, so an open compose panel stays open
          ui.query = "";
        }}
        class="min-w-0 flex-1 bg-transparent text-ink outline-none placeholder:text-ink-muted"
      />
    </label>
  </div>

  {#if list.length}
    <ul id="message-list" class="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
      {#each list as t (t.id)}
        {@const selected = ui.threadId === t.id}
        <li>
          <button
            id="thread-{t.id}"
            class="relative flex w-full flex-col gap-0.5 rounded-lg py-2.5 pr-3 pl-5 text-left transition-colors duration-100 {selected
              ? 'bg-selected focus-visible:outline-none'
              : 'hover:bg-raised/60 active:bg-raised'}"
            aria-current={selected ? "true" : undefined}
            onclick={() => openThread(t.id)}
            use:contextmenu={() => threadMenu(t, true)}
          >
            {#if t.unread}
              <span class="absolute top-[1.0625rem] left-2 size-1.5 rounded-full bg-accent"></span>
              <span class="sr-only">Unread.</span>
            {/if}
            <span class="flex items-center gap-2">
              <span class="min-w-0 flex-1 truncate {t.unread ? 'font-semibold' : 'font-medium'}">
                {t.participants.map((p) => p.name).join(", ")}
              </span>
              {#if ui.accountId === "all"}
                <span class="size-2 shrink-0 rounded-full" style:background={accountColor(t.accountId)}></span>
              {/if}
              <time datetime={t.lastDate} class="shrink-0 text-caption text-ink-muted">{formatDate(t.lastDate)}</time>
            </span>
            <span class="flex items-center gap-1.5">
              <span class="min-w-0 flex-1 truncate">{t.subject}</span>
              {#if t.hasAttachments}
                <Icon icon={Attachment01Icon} size={13} class="shrink-0 text-ink-muted" />
                <span class="sr-only">Has attachments.</span>
              {/if}
              {#if t.starred}
                <Icon icon={StarIcon} size={13} class="shrink-0 fill-accent text-accent" />
                <span class="sr-only">Starred.</span>
              {/if}
            </span>
            <span class="line-clamp-2 text-ink-muted">{t.snippet}</span>
          </button>
        </li>
      {/each}
    </ul>
  {:else if data.accounts.length === 0}
    <div class="grid flex-1 place-items-center px-6 text-center">
      <div class="flex flex-col items-center gap-2">
        <Icon icon={InboxIcon} size={28} class="text-ink-muted" />
        <p class="font-medium">No accounts yet</p>
        <p class="text-ink-muted">Add an IMAP account to bring its mail here.</p>
        <button class="btn-primary mt-3" onclick={() => (ui.addingAccount = true)}>Add account</button>
      </div>
    </div>
  {:else}
    <div class="grid flex-1 place-items-center text-ink-muted">
      <div class="flex flex-col items-center gap-2">
        <Icon icon={InboxIcon} size={28} />
        <p>{ui.query ? "No results" : "No messages"}</p>
      </div>
    </div>
  {/if}
</section>
