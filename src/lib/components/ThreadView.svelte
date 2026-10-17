<script lang="ts">
  import {
    Archive02Icon,
    Attachment01Icon,
    Delete02Icon,
    MailSend01Icon,
    MailReply01Icon,
    MailReplyAll01Icon,
    MoreHorizontalIcon,
    StarIcon,
  } from "@hugeicons/core-free-icons";
  import Icon from "./Icon.svelte";
  import { tick } from "svelte";
  import { prefersReducedMotion } from "svelte/motion";
  import { SvelteSet } from "svelte/reactivity";
  import Avatar from "./Avatar.svelte";
  import { archive, data, editDraft, forward, openAttachment, reply, toggleStar, trash, ui } from "$lib/state.svelte";
  import HtmlBody from "./HtmlBody.svelte";
  import Logo from "./Logo.svelte";
  import { formatDate, formatSize } from "$lib/format";
  import { splitQuoted, unquote } from "$lib/quote";
  import { mailFont } from "$lib/settings.svelte";
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

  // Opening a conversation brings its newest message (the open one, at the bottom of the trail) into view.
  let messages = $state<HTMLOListElement>();
  let arrived = false;
  const reveal = (el: Element | null | undefined) =>
    el?.scrollIntoView({ behavior: prefersReducedMotion.current ? "auto" : "smooth", block: "start" });
  $effect(() => {
    if (!thread || !messages || arrived) return;
    arrived = true;
    if (thread.messages.length > 1) reveal(messages.lastElementChild);
  });

  async function expand(m: Message) {
    toggle(expanded, m.id);
    if (!expanded.has(m.id)) return;
    await tick();
    reveal(document.getElementById(`message-${m.id}`));
  }
  // A long conversation shows its first message, how many are folded, and the last two (the newest open).
  let unfolded = $state(false);
  const rows = $derived.by(() => {
    const all = thread?.messages ?? [];
    if (unfolded || all.length <= 4) return all.map((m) => ({ m, folded: 0 }));
    return [{ m: all[0], folded: 0 }, { m: null, folded: all.length - 3 }, ...all.slice(-2).map((m) => ({ m, folded: 0 }))];
  });
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
        <h1 class="selectable text-title font-medium text-balance wrap-anywhere">{thread.subject}</h1>
        <p class="mt-1 text-caption text-ink-muted">
          {thread.messages.length}
          {thread.messages.length === 1 ? "message" : "messages"}
        </p>

        <!-- One flat list: hairlines separate messages. Everything a message holds starts where its sender's name does,
             past the avatar column (pl-11), so the eye follows one edge down the conversation. -->
        <ol bind:this={messages} class="mt-6 divide-y divide-line border-t border-line">
          {#each rows as { m, folded } (m?.id ?? "folded")}
            {#if !m}
              <li>
                <button
                  class="-mx-3 flex w-[calc(100%+1.5rem)] items-center gap-3 rounded-lg px-3 py-2 text-left text-ink-muted transition-colors duration-100 hover:bg-ink/5 hover:text-ink active:bg-ink/10"
                  onclick={() => (unfolded = true)}
                >
                  <span class="grid w-8 shrink-0 place-items-center">
                    <span class="grid h-5 min-w-6 place-items-center rounded-full border border-line px-1.5 text-caption tabular-nums"
                      >{folded}</span
                    >
                  </span>
                  {folded === 1 ? "1 more message" : `${folded} more messages`}
                </button>
              </li>
            {:else}
              {@const latest = m.id === thread.messages.at(-1)?.id}
              {@const open = latest || expanded.has(m.id)}
              {@const body = splitQuoted(m.body)}
              <li id="message-{m.id}" class="scroll-mt-3 {open ? 'pt-4 pb-6' : 'py-1'}">
                <h2>
                  {#if latest}
                    {@render header(m)}
                  {:else}
                    <button
                      class="-mx-3 block w-[calc(100%+1.5rem)] rounded-lg px-3 text-left transition-colors duration-100 hover:bg-ink/5 active:bg-ink/10 {open
                        ? 'py-1'
                        : 'py-2'}"
                      aria-expanded={open}
                      onclick={() => expand(m)}
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
                  <div class="selectable mt-4 pl-11 text-read wrap-anywhere">
                    {#if m.html}
                      <HtmlBody html={m.html} accountId={thread.accountId} />
                    {:else}
                      {#if body.text}
                        <p class="max-w-[68ch] whitespace-pre-wrap" style:font-family={mailFont().family}>{body.text}</p>
                      {/if}
                      {#if body.quoted && body.text}
                        <button
                          class="mt-3 grid h-6 w-9 place-items-center rounded-full bg-raised text-ink-muted transition-colors duration-100 hover:text-ink active:bg-ink/10"
                          aria-label="Show quoted text"
                          aria-expanded={quotesShown.has(m.id)}
                          onclick={() => toggle(quotesShown, m.id)}
                        >
                          <Icon icon={MoreHorizontalIcon} size={14} />
                        </button>
                      {/if}
                      {#if body.quoted && (!body.text || quotesShown.has(m.id))}
                        <p
                          class="mt-3 max-w-[68ch] border-l-2 border-line pl-3 whitespace-pre-wrap text-ink-muted"
                          style:font-family={mailFont().family}
                        >
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
                                class="flex items-center gap-1.5 rounded-full border border-line px-3 py-1 text-caption transition-colors duration-100 hover:bg-ink/5 active:bg-ink/10"
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
                    {:else if latest}
                      <!-- The conversation ends where you answer it. -->
                      <div class="mt-6 flex flex-wrap gap-2">
                        <button class="btn-secondary flex items-center gap-1.5" onclick={() => reply(thread.id)}>
                          <Icon icon={MailReply01Icon} size={15} /> Reply
                        </button>
                        {#if m.to.length + m.cc.length > 1}
                          <button class="btn-secondary flex items-center gap-1.5" onclick={() => reply(thread.id, true)}>
                            <Icon icon={MailReplyAll01Icon} size={15} /> Reply All
                          </button>
                        {/if}
                        <button class="btn-secondary flex items-center gap-1.5" onclick={() => forward(thread.id)}>
                          <Icon icon={MailSend01Icon} size={15} /> Forward
                        </button>
                      </div>
                    {/if}
                  </div>
                {/if}
              </li>
            {/if}
          {/each}
        </ol>
      </div>
    </div>
  </article>
{:else if !ui.threadId}
  <!-- The one decorative moment: the mark on a dotted grid, where there's nothing to read yet. -->
  <div data-tauri-drag-region class="dot-grid grid h-full place-items-center text-ink-muted">
    <div class="flex flex-col items-center gap-3 text-ink">
      <Logo size={56} />
      <p class="text-ink-muted">Select a conversation to read it</p>
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
