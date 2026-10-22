import { describe, expect, it } from "vitest";
import { addRecipient, emailsIn, matchContacts, pick, tokenAt } from "./contacts";

const ada = { name: "Ada Lovelace", email: "ada.l@x.example" };
const bob = { name: "Bob Stone", email: "bob@x.example" };
const jane = { name: "Doe, Jane", email: "jane@x.example" };
const all = [bob, ada, jane];

describe("matchContacts", () => {
  it("matches the start of a name's words, the address, or a piece of its local part", () => {
    expect(matchContacts(all, "lov", [])).toEqual([ada]);
    expect(matchContacts(all, "ADA", [])).toEqual([ada]);
    expect(matchContacts(all, "l", [])).toEqual([ada]); // ada.l
    expect(matchContacts(all, "ace", [])).toEqual([]);
    expect(matchContacts(all, "", [])).toEqual([]);
  });
  it("keeps the engine's order and leaves out who's already there", () => {
    expect(matchContacts(all, "x", [])).toEqual([]);
    expect(matchContacts([bob, ada], "b", [])).toEqual([bob]);
    expect(matchContacts(all, "ada", ["ada.l@x.example"])).toEqual([]);
    expect(matchContacts(Array(20).fill(ada), "a", []).length).toBe(8);
  });
});

describe("address fields", () => {
  it("finds the piece being typed", () => {
    expect(tokenAt("Bob <b@x>, ad", 13)).toEqual({ start: 10, end: 13, query: "ad" });
    expect(tokenAt("ad, Bob <b@x>", 2)).toEqual({ start: 0, end: 2, query: "ad" });
  });
  it("puts the pick in its place, then room for the next", () => {
    expect(pick("Bob <b@x>, ad", 13, ada)).toEqual({ value: "Bob <b@x>, Ada Lovelace <ada.l@x.example>, ", caret: 43 });
    expect(pick("ad, Bob <b@x>", 2, ada).value).toBe("Ada Lovelace <ada.l@x.example>, Bob <b@x>");
    expect(pick("do", 2, jane).value).toBe('"Doe, Jane" <jane@x.example>, ');
  });
  it("reads the addresses typed, and adds someone only once", () => {
    expect(emailsIn('Ada <ADA.l@x.example>, bob@x.example, "Doe, Jane" <jane@x.example>')).toEqual([
      "ada.l@x.example",
      "bob@x.example",
      "jane@x.example",
    ]);
    expect(addRecipient("", ada)).toBe("Ada Lovelace <ada.l@x.example>");
    expect(addRecipient("bob@x.example", ada)).toBe("bob@x.example, Ada Lovelace <ada.l@x.example>");
    expect(addRecipient("bob@x.example, ", ada)).toBe("bob@x.example, Ada Lovelace <ada.l@x.example>");
    expect(addRecipient("Ada <ADA.L@x.example>", ada)).toBe("Ada <ADA.L@x.example>");
  });
});
