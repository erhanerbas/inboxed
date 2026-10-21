import { describe, expect, it } from "vitest";
import { fromDomains, notices, summarize } from "./notify";
import type { Account, NewMail } from "./types";

const acc = (id: string, name: string): Account => ({ id, name, email: `${id}@x`, provider: "imap", color: "" });
const mail = (from: string, subject = "Dinner plans", snippet = "Are you free?", address = "maya@northwind.studio"): NewMail => ({
  accountId: "a1", threadId: `a1/${from}`, from, address, subject, snippet,
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

describe("fromDomains", () => {
  const from = (address: string) => mail("Maya Chen", "Hi", "", address);

  it("nothing listed: every sender", () => {
    expect(fromDomains(from("maya@anywhere.com"), " \n")).toBe(true);
  });

  it("the listed domains and their subdomains, however they're written", () => {
    const domains = "Northwind.Studio, *.halden.shop\n@nordlys.hotels";
    expect(fromDomains(from("maya@northwind.studio"), domains)).toBe(true);
    expect(fromDomains(from("MAYA@mail.Northwind.studio"), domains)).toBe(true);
    expect(fromDomains(from("orders@eu.halden.shop"), domains)).toBe(true);
    expect(fromDomains(from("ingrid@nordlys.hotels"), domains)).toBe(true);
    expect(fromDomains(from("sam@notnorthwind.studio"), domains)).toBe(false);
    expect(fromDomains(from("sam@example.com"), domains)).toBe(false);
  });
});

describe("notices", () => {
  it("one per message from the chosen domains, carrying the mail its click opens", () => {
    const keep = mail("Maya Chen");
    const out = notices([keep, mail("Sam Lee", "Hi", "", "sam@example.com")], [acc("a1", "Home")], "northwind.studio");
    expect(out).toEqual([{ title: "Maya Chen", body: "Dinner plans\nAre you free?", mail: keep }]);
  });

  it("past ten, the rest share one", () => {
    const batch = Array.from({ length: 13 }, (_, i) => mail(`Sender ${i}`));
    const out = notices(batch, [acc("a1", "Home")], "");
    expect(out).toHaveLength(11);
    expect(out[10]).toEqual({ title: "3 new messages", body: "Sender 10, Sender 11 and 1 more" });
  });
});
