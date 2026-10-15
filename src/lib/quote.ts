const ATTRIBUTION = /^On .+ wrote:\s*$/;
const OUTLOOK_MARKER = /^-{2,}\s*Original Message\s*-{2,}$/i;

/** Splits a plain-text body into what the sender wrote and the quoted reply chain below it. */
export function splitQuoted(body: string): { text: string; quoted: string } {
  const lines = body.split("\n");
  const start = lines.findIndex(
    (line, i) =>
      ATTRIBUTION.test(line) ||
      OUTLOOK_MARKER.test(line.trim()) ||
      (line.startsWith(">") && lines.slice(i).every((l) => l.startsWith(">") || l.trim() === "")),
  );
  if (start === -1) return { text: body, quoted: "" };
  return { text: lines.slice(0, start).join("\n").trimEnd(), quoted: lines.slice(start).join("\n") };
}
