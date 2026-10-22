<script lang="ts">
  import { openUrl } from "@tauri-apps/plugin-opener";
  import { ArrowLeft01Icon, ArrowRight01Icon, Cancel01Icon, File01Icon } from "@hugeicons/core-free-icons";
  import { fetchFile } from "$lib/engine";
  import { formatSize } from "$lib/format";
  import { previewDoc, previewKind } from "$lib/preview";
  import { data, openAttachment, ui } from "$lib/state.svelte";
  import Icon from "./Icon.svelte";

  // A received attachment shown in Inboxed, to decide whether to keep it. Save to Downloads is the old open.
  let dialog = $state<HTMLDialogElement>();
  $effect(() => dialog?.showModal());

  const message = $derived(data.thread?.messages.find((m) => m.id === ui.preview?.messageId));
  const files = $derived(message?.attachments.flatMap((a, index) => (a.inline ? [] : [{ a, index }])) ?? []);
  const at = $derived(files.findIndex((f) => f.index === ui.preview?.index));
  const file = $derived(files[at]?.a);
  const kind = $derived(file ? previewKind(file) : null);

  type View = { state: "loading" | "error" | "ready"; url?: string; text?: string; html?: string; sheets?: { name: string; html: string }[] };
  let view = $state<View>({ state: "loading" });
  let sheet = $state(0);

  // Fetched (with the token, never in a URL) and converted here; documents end up in a sandboxed frame.
  $effect(() => {
    const p = ui.preview;
    const k = kind;
    if (!p || !k) return;
    let gone = false;
    let url = "";
    view = { state: "loading" };
    sheet = 0;
    (async () => {
      const blob = await fetchFile(`/messages/${encodeURIComponent(p.messageId)}/attachments/${p.index}`);
      if (k === "pdf" || k === "image") {
        url = URL.createObjectURL(k === "pdf" ? new Blob([blob], { type: "application/pdf" }) : blob);
        return { url };
      }
      if (k === "text") return { text: await blob.text() };
      if (k === "docx") {
        const { renderAsync } = await import("docx-preview");
        // Built in a document of its own: nothing in it is live in the app's page.
        const box = document.implementation.createHTMLDocument().body;
        await renderAsync(blob, box, box, { inWrapper: true, useBase64URL: true, ignoreLastRenderedPageBreak: true });
        return { html: box.innerHTML };
      }
      const XLSX = await import("xlsx");
      const book = XLSX.read(await blob.arrayBuffer());
      return { sheets: book.SheetNames.map((name) => ({ name, html: XLSX.utils.sheet_to_html(book.Sheets[name], { header: "", footer: "" }) })) };
    })().then(
      (ready) => {
        if (!gone) view = { state: "ready", ...ready };
        else if (url) URL.revokeObjectURL(url);
      },
      (err) => {
        console.error("preview", err);
        if (!gone) view = { state: "error" };
      },
    );
    return () => {
      gone = true;
      if (url) URL.revokeObjectURL(url);
    };
  });

  const go = (step: number) => {
    const next = files[(at + step + files.length) % files.length];
    if (next && ui.preview) ui.preview = { messageId: ui.preview.messageId, index: next.index };
  };

  // Links in a document go to the browser (http/https only); nothing navigates the frame.
  function frameLoaded(e: Event) {
    const doc = (e.currentTarget as HTMLIFrameElement).contentDocument;
    doc?.addEventListener("click", (click) => {
      const a = (click.target as Element).closest?.("a[href]");
      if (!a) return;
      click.preventDefault();
      const href = a.getAttribute("href") ?? "";
      if (!/^https?:\/\//i.test(href)) return;
      if ("__TAURI_INTERNALS__" in window) openUrl(href).catch(console.error);
      else window.open(href, "_blank", "noopener");
    });
  }
</script>

<dialog
  bind:this={dialog}
  onclose={() => (ui.preview = null)}
  onkeydown={(e) => {
    if (files.length > 1 && (e.key === "ArrowLeft" || e.key === "ArrowRight") && !(e.target as Element).closest("select, input")) {
      e.preventDefault();
      go(e.key === "ArrowRight" ? 1 : -1);
    }
  }}
  aria-label={file ? `Preview of ${file.name}` : "Preview"}
  class="m-auto flex h-[calc(100vh-3rem)] w-[min(64rem,calc(100vw-3rem))] max-w-none flex-col overflow-hidden rounded-2xl border border-line bg-float text-ink shadow-float backdrop:bg-ink/30"
>
  <header class="flex h-12 shrink-0 items-center gap-2 border-b border-line pr-2 pl-4">
    <div class="min-w-0 flex-1">
      <h2 class="truncate font-semibold">{file?.name ?? "Attachment"}</h2>
    </div>
    {#if file}<span class="shrink-0 text-caption text-ink-muted tabular-nums">{formatSize(file.size)}</span>{/if}
    {#if files.length > 1}
      <span class="flex shrink-0 items-center">
        <button class="icon-btn" aria-label="Previous attachment" title="Previous (←)" onclick={() => go(-1)}><Icon icon={ArrowLeft01Icon} size={16} /></button>
        <span class="text-caption text-ink-muted tabular-nums">{at + 1} of {files.length}</span>
        <button class="icon-btn" aria-label="Next attachment" title="Next (→)" onclick={() => go(1)}><Icon icon={ArrowRight01Icon} size={16} /></button>
      </span>
    {/if}
    {#if ui.preview}
      {@const p = ui.preview}
      <button class="btn-secondary ml-1 shrink-0" title="Save to Downloads and show it" onclick={() => openAttachment(p.messageId, p.index)}>
        Save to Downloads
      </button>
    {/if}
    <button class="icon-btn" aria-label="Close" title="Close (Esc)" onclick={() => dialog?.close()}><Icon icon={Cancel01Icon} size={16} /></button>
  </header>

  {#if view.state === "ready" && view.sheets && view.sheets.length > 1}
    <div role="tablist" aria-label="Sheets" class="flex shrink-0 gap-1 overflow-x-auto border-b border-line px-3">
      {#each view.sheets as s, i (i)}
        <button
          role="tab"
          aria-selected={sheet === i}
          class="-mb-px shrink-0 border-b-2 px-2.5 py-2 transition-colors duration-100 active:text-ink {sheet === i
            ? 'border-accent font-medium text-ink'
            : 'border-transparent text-ink-muted hover:text-ink'}"
          onclick={() => (sheet = i)}
        >
          {s.name}
        </button>
      {/each}
    </div>
  {/if}

  <div class="relative min-h-0 flex-1 bg-raised">
    {#if !kind || view.state === "error"}
      <div class="grid h-full place-items-center p-8 text-center">
        <div class="flex flex-col items-center gap-3">
          <Icon icon={File01Icon} size={40} class="text-ink-muted" />
          <p class="font-medium">{file?.name}</p>
          <p class="text-ink-muted">{kind ? "Inboxed couldn't show this file." : "There's no preview for this kind of file."}</p>
          {#if ui.preview}
            {@const p = ui.preview}
            <button class="btn-primary mt-2" onclick={() => openAttachment(p.messageId, p.index)}>Save to Downloads</button>
          {/if}
        </div>
      </div>
    {:else if view.state === "loading"}
      <p class="grid h-full place-items-center text-ink-muted">Opening…</p>
    {:else if view.url && kind === "pdf"}
      <iframe src={view.url} title={file?.name} class="size-full border-0"></iframe>
    {:else if view.url}
      <img src={view.url} alt={file?.name} class="size-full object-contain p-6" />
    {:else if view.text !== undefined}
      <pre class="size-full overflow-auto bg-canvas p-6 font-mono text-ui whitespace-pre-wrap selectable">{view.text}</pre>
    {:else}
      {@const html = view.html ?? view.sheets?.[sheet]?.html ?? ""}
      <!-- allow-same-origin only: no scripts, forms, popups or navigation; the page catches link clicks. -->
      <iframe sandbox="allow-same-origin" srcdoc={previewDoc(html)} title={file?.name} onload={frameLoaded} class="size-full border-0 bg-white"></iframe>
    {/if}
  </div>
</dialog>
