import { describe, expect, it } from "vitest";
import { summarize } from "./notify";
import type { Account, NewMail } from "./types";

const acc = (id: string, name: string): Account => ({ id, name, email: `${id}@x`, provider: "imap", color: "" });
const mail = (from: string, subject = "Dinner plans", snippet = "Are you free?"): NewMail => ({
  accountId: "a1", threadId: `a1/${from}`, from, subject, snippet,
});

describe("summarize", () => {
  it("one message: who, then what", () => {
    expect(summarize([mail("Maya Chen")], [acc("a1", "Home")])).toEqual({ title: "Maya Chen", body: "Dinner plans\nAre you free?" });
  });

  it("several: how many and from whom, in one notification", () => {
    expect(summarize([mail("Maya Chen"), mail("Sam Lee"), mail("Lee Park")], [acc("a1", "Home")])).toEqual({
      title: "3 new messages",
      body: "Maya Chen, Sam Lee and 1 more",
    });
    expect(summarize([mail("Maya Chen"), mail("Sam Lee")], [acc("a1", "Home")]).body).toBe("Maya Chen and Sam Lee");
    expect(summarize([mail("Maya Chen"), mail("Maya Chen")], [acc("a1", "Home")]).body).toBe("Maya Chen");
  });

  it("names the account when there are several", () => {
    expect(summarize([mail("Maya Chen")], [acc("a1", "Home"), acc("a2", "Work")]).title).toBe("Maya Chen · Home");
  });
});
