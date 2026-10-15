<script lang="ts">
  import Inbox from "@lucide/svelte/icons/inbox";
  import Paperclip from "@lucide/svelte/icons/paperclip";
  import Search from "@lucide/svelte/icons/search";
  import Star from "@lucide/svelte/icons/star";
  import { data, ui, openThread } from "$lib/state.svelte";
  import { threadsFor, visibleMailboxes } from "$lib/views";
  import { formatDate } from "$lib/format";

  const list = $derived(threadsFor(data.threads, data.mailboxes, ui.accountId, ui.mailboxKey, ui.query));
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
  <header data-tauri-drag-region class="flex h-13 shrink-0 items-center justify-between gap-3 px-4">
    <h1 class="truncate text-heading">{title}</h1>
    <span class="shrink-0 text-caption text-ink-muted">
      {list.length}
      {list.length === 1 ? "conversation" : "conversations"}
    </span>
  </header>

  <div class="px-3 pb-2">
    <label class="flex h-8 items-center gap-2 rounded-lg bg-raised px-2.5 text-ink-muted focus-within:ring-2 focus-within:ring-accent">
      <Search size={14} />
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
                <Paperclip size={13} class="shrink-0 text-ink-muted" />
                <span class="sr-only">Has attachments.</span>
              {/if}
              {#if t.starred}
                <Star size={13} class="shrink-0 fill-accent text-accent" />
                <span class="sr-only">Starred.</span>
              {/if}
            </span>
            <span class="line-clamp-2 text-ink-muted">{t.snippet}</span>
          </button>
        </li>
      {/each}
    </ul>
  {:else}
    <div class="grid flex-1 place-items-center text-ink-muted">
      <div class="flex flex-col items-center gap-2">
        <Inbox size={28} />
        <p>{ui.query ? "No results" : "No messages"}</p>
      </div>
    </div>
  {/if}
</section>
