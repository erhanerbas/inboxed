// Builds the Go engine into the path Tauri expects for a sidecar:
// src-tauri/binaries/inboxed-engine-<target-triple>[.exe]
import { execSync } from "node:child_process";

const triple = execSync("rustc --print host-tuple").toString().trim();
const ext = process.platform === "win32" ? ".exe" : "";
execSync(`go build -o ../src-tauri/binaries/inboxed-engine-${triple}${ext} .`, {
  cwd: "engine",
  stdio: "inherit",
});
