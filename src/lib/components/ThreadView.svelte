<script lang="ts">
  import Archive from "@lucide/svelte/icons/archive";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import Forward from "@lucide/svelte/icons/forward";
  import Mail from "@lucide/svelte/icons/mail";
  import Paperclip from "@lucide/svelte/icons/paperclip";
  import Reply from "@lucide/svelte/icons/reply";
  import ReplyAll from "@lucide/svelte/icons/reply-all";
  import Star from "@lucide/svelte/icons/star";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import { SvelteSet } from "svelte/reactivity";
  import Avatar from "./Avatar.svelte";
  import { data, ui } from "$lib/state.svelte";
  import { formatDate, formatSize } from "$lib/format";
  import { splitQuoted } from "$lib/quote";
  import type { Address, Message } from "$lib/types";

  // Archive, delete, reply and forward get wired up in later phases.
  // Grouped by what they do to the conversation: file it away, or answer it.
  const actionGroups = [
    [
      { label: "Archive", icon: Archive },
      { label: "Delete", icon: Trash2 },
    ],
    [
      { label: "Reply", icon: Reply },
      { label: "Reply all", icon: ReplyAll },
      { label: "Forward", icon: Forward },
    ],
  ];

  const thread = $derived(data.threads.find((t) => t.id === ui.threadId));
  // +page.svelte re-creates this component per thread, so both sets start empty.
  const expanded = new SvelteSet<string>();
  const quotesShown = new SvelteSet<string>();

  function toggle(set: SvelteSet<string>, id: string) {
    if (set.has(id)) set.delete(id);
    else set.add(id);
  }
  const names = (list: Address[]) => list.map((a) => a.name).join(", ");
</script>

{#if thread}
  <article class="flex h-full flex-col">
    <header data-tauri-drag-region class="flex h-13 shrink-0 items-center justify-end gap-4 border-b border-line px-3">
      {#each actionGroups as group, g (g)}
        <div class="flex gap-0.5">
          {#each group as a (a.label)}
            <button class="icon-btn" aria-label={a.label} title={a.label}><a.icon size={17} /></button>
          {/each}
        </div>
      {/each}
      <button
        class="icon-btn"
        aria-label="Star"
        title="Star"
        aria-pressed={thread.starred}
        onclick={() => (thread.starred = !thread.starred)}
      >
        <Star size={17} class={thread.starred ? "fill-accent text-accent" : ""} />
      </button>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="mx-auto max-w-3xl px-8 pt-6 pb-10">
        <h1 class="selectable text-title font-semibold wrap-anywhere">{thread.subject}</h1>
        <p class="mt-1 text-caption text-ink-muted">
          {thread.messages.length}
          {thread.messages.length === 1 ? "message" : "messages"}
        </p>

        <ol class="mt-6 flex flex-col gap-2">
          {#each thread.messages as m, i (m.id)}
            {@const latest = i === thread.messages.length - 1}
            {@const open = latest || expanded.has(m.id)}
            {@const body = splitQuoted(m.body)}
            <li class="rounded-xl border border-line bg-panel">
              {#if latest}
                <div class="flex items-start gap-3 px-5 pt-4">{@render head(m, open)}</div>
              {:else}
                <button
                  class="flex w-full items-start gap-3 rounded-xl px-5 text-left transition-colors duration-100 hover:bg-raised/40 active:bg-raised/60 {open
                    ? 'pt-4'
                    : 'py-3'}"
                  aria-expanded={open}
                  onclick={() => toggle(expanded, m.id)}
                >
                  {@render head(m, open)}
                </button>
              {/if}

              {#if open}
                <div class="selectable pt-3 pr-5 pb-5 pl-16 text-read wrap-anywhere">
                  {#if body.text}
                    <p class="whitespace-pre-wrap">{body.text}</p>
                  {/if}
                  {#if body.quoted && body.text}
                    <button
                      class="mt-2 grid h-5 w-8 place-items-center rounded-full bg-raised text-ink-muted hover:text-ink"
                      aria-label="Show quoted text"
                      aria-expanded={quotesShown.has(m.id)}
                      onclick={() => toggle(quotesShown, m.id)}
                    >
                      <Ellipsis size={14} />
                    </button>
                  {/if}
                  {#if body.quoted && (!body.text || quotesShown.has(m.id))}
                    <p class="mt-3 border-l-2 border-line pl-3 whitespace-pre-wrap text-ink-muted">{body.quoted}</p>
                  {/if}
                  {#if m.attachments.length}
                    <ul class="mt-4 flex flex-wrap gap-2">
                      {#each m.attachments as a (a.name)}
                        <li class="flex items-center gap-1.5 rounded-lg bg-raised px-2.5 py-1.5 text-caption">
                          <Paperclip size={12} class="text-ink-muted" />
                          {a.name}
                          <span class="text-ink-muted">{formatSize(a.size)}</span>
                        </li>
                      {/each}
                    </ul>
                  {/if}
                </div>
              {/if}
            </li>
          {/each}
        </ol>
      </div>
    </div>
  </article>
{:else}
  <div data-tauri-drag-region class="grid h-full place-items-center text-ink-muted">
    <div class="flex flex-col items-center gap-2">
      <Mail size={28} />
      <p>No conversation selected</p>
    </div>
  </div>
{/if}

{#snippet head(m: Message, open: boolean)}
  <!-- Fixed-width slot so names line up whether the message is open or collapsed. -->
  <span class="grid w-8 shrink-0 place-items-center"><Avatar name={m.from.name} size={open ? 32 : 24} /></span>
  <div class="min-w-0 flex-1">
    <div class="flex items-baseline gap-2">
      <span class="truncate font-semibold">{m.from.name}</span>
      {#if open}
        <span class="truncate text-caption text-ink-muted">{m.from.email}</span>
      {/if}
      <time datetime={m.date} class="ml-auto shrink-0 text-caption text-ink-muted">{formatDate(m.date)}</time>
    </div>
    {#if open}
      <p class="truncate text-caption text-ink-muted">
        To {names(m.to)}{m.cc.length ? ` · Cc ${names(m.cc)}` : ""}
      </p>
    {:else}
      <p class="truncate text-ink-muted">{(splitQuoted(m.body).text || m.body).replace(/\s+/g, " ")}</p>
    {/if}
  </div>
{/snippet}
