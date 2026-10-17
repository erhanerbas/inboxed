import { beforeEach, describe, expect, it, vi } from "vitest";

// The tests run in Node: a Map stands in for the browser's storage.
const store = new Map<string, string>();
vi.stubGlobal("localStorage", {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => store.set(k, v),
});

describe("mail font", () => {
  beforeEach(() => {
    vi.resetModules();
    store.clear();
  });

  it("is Inter until the person picks one, and remembers the pick", async () => {
    const { mailFont, setMailFont } = await import("./settings.svelte");
    expect(mailFont().name).toBe("Inter");
    setMailFont("garamond");
    expect(mailFont().name).toBe("EB Garamond");
    vi.resetModules();
    expect((await import("./settings.svelte")).mailFont().name).toBe("EB Garamond");
  });

  it("ignores fonts it doesn't know", async () => {
    localStorage.setItem("inboxed.mailFont", "comic-sans");
    const { mailFont, setMailFont } = await import("./settings.svelte");
    expect(mailFont().name).toBe("Inter");
    setMailFont("comic-sans");
    expect(mailFont().name).toBe("Inter");
  });
});
