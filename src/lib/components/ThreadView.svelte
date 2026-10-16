<script lang="ts">
  import {
    Archive02Icon,
    Attachment01Icon,
    Delete02Icon,
    MailSend01Icon,
    Mail01Icon,
    MailReply01Icon,
    MailReplyAll01Icon,
    MoreHorizontalIcon,
    StarIcon,
  } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { SvelteSet } from "svelte/reactivity";
  import Avatar from "./Avatar.svelte";
  import { archive, data, editDraft, forward, openAttachment, reply, toggleStar, trash, ui } from "$lib/state.svelte";
  import HtmlBody from "./HtmlBody.svelte";
  import { formatDate, formatSize } from "$lib/format";
  import { splitQuoted, unquote } from "$lib/quote";
  import type { Address, Message } from "$lib/types";

  // Grouped by what they do to the conversation: file it away, or answer it. Labels match the right-click menu.
  const actionGroups = [
    [
      { label: "Archive", keys: "⌃⌘A", icon: Archive02Icon, run: archive },
      { label: "Move to Trash", keys: "⌫", icon: Delete02Icon, run: trash },
    ],
    [
      { label: "Reply", keys: "⌘R", icon: MailReply01Icon, run: (id: string) => reply(id) },
      { label: "Reply All", keys: "⇧⌘R", icon: MailReplyAll01Icon, run: (id: string) => reply(id, true) },
      { label: "Forward", keys: "⇧⌘F", icon: MailSend01Icon, run: forward },
    ],
  ];

  // data.thread can briefly be the previous conversation while the next one loads.
  const thread = $derived(data.thread?.id === ui.threadId ? data.thread : null);
  // +page.svelte re-creates this component per thread, so both sets start empty.
  const expanded = new SvelteSet<string>();
  const quotesShown = new SvelteSet<string>();

  function toggle(set: SvelteSet<string>, id: string) {
    if (set.has(id)) set.delete(id);
    else set.add(id);
  }
  const names = (list: Address[]) => list.map((a) => a.name).join(", ");
  const preview = (m: Message) => (splitQuoted(m.body).text || m.body).replace(/\s+/g, " ");
</script>

{#if thread}
  <article class="flex h-full flex-col">
    <header data-tauri-drag-region class="flex h-13 shrink-0 items-center justify-end gap-4 border-b border-line px-3">
      {#each actionGroups as group, g (g)}
        <div class="flex gap-0.5">
          {#each group as a (a.label)}
            <button class="icon-btn" aria-label={a.label} title="{a.label} ({a.keys})" onclick={() => a.run(thread.id)}>
              <Icon icon={a.icon} size={17} />
            </button>
          {/each}
        </div>
      {/each}
      <button
        class="icon-btn"
        aria-label="Star"
        title="Star (⇧⌘L)"
        aria-pressed={thread.starred}
        onclick={() => toggleStar(thread.id)}
      >
        <Icon icon={StarIcon} size={17} class={thread.starred ? "fill-accent text-accent" : ""} />
      </button>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto">
      <div class="mx-auto max-w-3xl px-8 pt-6 pb-10">
        <h1 class="selectable text-title font-semibold text-balance wrap-anywhere">{thread.subject}</h1>
        <p class="mt-1 text-caption text-ink-muted">
          {thread.messages.length}
          {thread.messages.length === 1 ? "message" : "messages"}
        </p>

        <!-- One flat list: hairlines separate messages, space separates the parts of one message. -->
        <ol class="mt-6 divide-y divide-line border-t border-line">
          {#each thread.messages as m, i (m.id)}
            {@const latest = i === thread.messages.length - 1}
            {@const open = latest || expanded.has(m.id)}
            {@const body = splitQuoted(m.body)}
            <li class={open ? "pt-4 pb-6" : "py-1"}>
              <h2>
                {#if latest}
                  {@render header(m)}
                {:else}
                  <button
                    class="-mx-3 block w-[calc(100%+1.5rem)] rounded-lg px-3 text-left transition-colors duration-100 hover:bg-raised/40 active:bg-raised/60 {open
                      ? 'py-1'
                      : 'py-2'}"
                    aria-expanded={open}
                    onclick={() => toggle(expanded, m.id)}
                  >
                    {#if open}
                      {@render header(m)}
                    {:else}
                      <span class="flex items-center gap-3">
                        <span class="grid w-8 shrink-0 place-items-center"><Avatar name={m.from.name} size={24} /></span>
                        <span class="max-w-[40%] shrink-0 truncate font-semibold">{m.from.name}</span>
                        <span class="min-w-0 flex-1 truncate text-ink-muted">{preview(m)}</span>
                        <time datetime={m.date} class="shrink-0 text-caption text-ink-muted">{formatDate(m.date)}</time>
                      </span>
                    {/if}
                  </button>
                {/if}
              </h2>

              {#if open}
                <div class="selectable mt-4 text-read wrap-anywhere">
                  {#if m.html}
                    <HtmlBody html={m.html} accountId={thread.accountId} />
                  {:else}
                  {#if body.text}
                    <p class="max-w-[68ch] whitespace-pre-wrap">{body.text}</p>
                  {/if}
                  {#if body.quoted && body.text}
                    <button
                      class="mt-3 grid h-6 w-9 place-items-center rounded-full bg-raised text-ink-muted transition-colors duration-100 hover:text-ink active:bg-raised/60"
                      aria-label="Show quoted text"
                      aria-expanded={quotesShown.has(m.id)}
                      onclick={() => toggle(quotesShown, m.id)}
                    >
                      <Icon icon={MoreHorizontalIcon} size={14} />
                    </button>
                  {/if}
                  {#if body.quoted && (!body.text || quotesShown.has(m.id))}
                    <p class="mt-3 max-w-[68ch] border-l-2 border-line pl-3 whitespace-pre-wrap text-ink-muted">
                      {unquote(body.quoted)}
                    </p>
                  {/if}
                  {/if}
                  {#if m.attachments.some((a) => !a.inline)}
                    <ul class="mt-4 flex flex-wrap gap-2" aria-label="Attachments">
                      {#each m.attachments as a, i (i)}
                        {#if !a.inline}
                          <li>
                            <button
                              class="flex items-center gap-1.5 rounded-lg bg-raised px-2.5 py-1.5 text-caption transition-colors duration-100 hover:bg-raised/70 active:bg-selected"
                              title="Save to Downloads and show it"
                              onclick={() => openAttachment(m.id, i)}
                            >
                              <Icon icon={Attachment01Icon} size={12} class="text-ink-muted" />
                              {a.name}
                              <span class="text-ink-muted">{formatSize(a.size)}</span>
                            </button>
                          </li>
                        {/if}
                      {/each}
                    </ul>
                  {/if}
                  {#if m.draft}
                    <button class="btn-secondary mt-4" onclick={() => editDraft(m)}>Edit draft</button>
                  {/if}
                </div>
              {/if}
            </li>
          {/each}
        </ol>
      </div>
    </div>
  </article>
{:else if !ui.threadId}
  <div data-tauri-drag-region class="grid h-full place-items-center text-ink-muted">
    <div class="flex flex-col items-center gap-2">
      <Icon icon={Mail01Icon} size={28} />
      <p>Select a conversation to read it</p>
    </div>
  </div>
{/if}

<!-- Header of an open message. Spans only: it can sit inside a button. Sender and recipients wrap rather than truncate. -->
{#snippet header(m: Message)}
  <span class="flex items-start gap-3">
    <Avatar name={m.from.name} size={32} />
    <span class="block min-w-0 flex-1">
      <span class="flex items-baseline gap-3">
        <span class="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
          <span class="font-semibold">{m.from.name}</span>
          <span class="text-caption text-ink-muted wrap-anywhere">{m.from.email}</span>
        </span>
        {#if m.draft}<span class="shrink-0 text-caption font-medium text-ink-muted">Draft</span>{/if}
        <time datetime={m.date} class="shrink-0 text-caption text-ink-muted">{formatDate(m.date)}</time>
      </span>
      <span class="block text-caption text-ink-muted">
        To {names(m.to)}{m.cc.length ? ` · Cc ${names(m.cc)}` : ""}
      </span>
    </span>
  </span>
{/snippet}
