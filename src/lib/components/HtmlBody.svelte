<script lang="ts">
  import { MoreHorizontalIcon } from "@hugeicons/core-free-icons";
  import { openUrl } from "@tauri-apps/plugin-opener";
  import Icon from "./Icon.svelte";
  import { frameDoc, hasRemoteContent, QUOTE_SELECTOR } from "$lib/html";
  import { compose } from "$lib/state.svelte";

  let { html }: { html: string } = $props();

  let frame = $state<HTMLIFrameElement>();
  let remote = $state(false); // per message, per visit: images load only when asked
  let height = $state(0);
  let hasQuotes = $state(false);
  let showQuotes = $state(false);
  const doc = $derived(frameDoc(html, remote));
  let observer: ResizeObserver | undefined;

  // The frame runs no scripts, so it can't size itself: the page measures its content.
  function measure() {
    const body = frame?.contentDocument?.body;
    if (body) height = Math.ceil(body.getBoundingClientRect().height);
  }

  function loaded() {
    const d = frame?.contentDocument;
    if (!d) return;
    observer?.disconnect();
    observer = new ResizeObserver(measure);
    observer.observe(d.body);
    d.addEventListener("load", measure, true); // images arriving change the height
    d.addEventListener("click", follow);
    hasQuotes = !!d.querySelector(QUOTE_SELECTOR);
    d.documentElement.classList.toggle("ib-show-quotes", showQuotes);
    measure();
  }
  $effect(() => () => observer?.disconnect());

  // Links never open inside the frame: web links go to the browser, mailto opens compose, anything else is ignored.
  function follow(e: MouseEvent) {
    const link = (e.target as Element | null)?.closest?.("a[href]") as HTMLAnchorElement | null;
    if (!link) return;
    e.preventDefault();
    let url: URL;
    try {
      url = new URL(link.href);
    } catch {
      return;
    }
    if (url.protocol === "mailto:") {
      compose({ to: decodeURIComponent(url.pathname), subject: url.searchParams.get("subject") ?? "" });
    } else if (url.protocol === "https:" || url.protocol === "http:") {
      if ("__TAURI_INTERNALS__" in window) openUrl(url.href);
      else window.open(url.href, "_blank", "noopener");
    }
  }

  function toggleQuotes() {
    showQuotes = !showQuotes;
    frame?.contentDocument?.documentElement.classList.toggle("ib-show-quotes", showQuotes);
    measure();
  }
</script>

{#if hasRemoteContent(html) && !remote}
  <p class="mb-3 flex max-w-[68ch] items-center justify-between gap-3 rounded-lg bg-panel py-1.5 pr-1.5 pl-3 text-caption text-ink-muted">
    Remote images are blocked, so the sender can't tell you opened this.
    <button class="btn-secondary h-7 shrink-0 px-3 text-caption" onclick={() => (remote = true)}>Load images</button>
  </p>
{/if}
<!-- allow-same-origin only: no scripts, forms, popups or navigation; the page can measure it and catch clicks. -->
<iframe
  bind:this={frame}
  title="Message"
  sandbox="allow-same-origin"
  srcdoc={doc}
  onload={loaded}
  class="block w-full rounded-xl bg-white"
  style:height="{height}px"
></iframe>
{#if hasQuotes}
  <button
    class="mt-3 grid h-6 w-9 place-items-center rounded-full bg-raised text-ink-muted transition-colors duration-100 hover:text-ink active:bg-raised/60"
    aria-label={showQuotes ? "Hide quoted text" : "Show quoted text"}
    aria-expanded={showQuotes}
    onclick={toggleQuotes}
  >
    <Icon icon={MoreHorizontalIcon} size={14} />
  </button>
{/if}
