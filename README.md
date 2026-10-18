# Inboxed

A desktop email client for all your accounts in one place. Built with Tauri, SvelteKit, Tailwind and Go.

- One unified inbox across every account, or one account at a time
- Two-way IMAP sync: read, star, archive and trash show up on your other devices
- New-mail notifications and a Dock badge
- Compose, reply, forward and drafts over SMTP
- HTML mail shown safely: no scripts, and remote images blocked until you load them
- Full-text search, light and dark themes

## Status

Early and macOS-only. Accounts connect over IMAP and SMTP with a password. Gmail, Yahoo and iCloud need an [app password](https://support.google.com/accounts/answer/185833). Personal Outlook.com accounts can't connect, since Microsoft turned off password sign-in for them.

Your password is kept in the macOS keychain. Mail is stored in a local SQLite database in `~/Library/Application Support/dev.inboxed.app`. Nothing goes anywhere except your mail servers.

## Requirements

Node, Rust, Go, and the [Tauri prerequisites](https://v2.tauri.app/start/prerequisites/).

## Develop

```sh
npm install
npm run tauri dev       # run the app (builds the Go engine first)
npm run tauri build     # production bundle
```

To work on the UI in a plain browser with demo mail, run the engine with seed data next to Vite:

```sh
npm run dev:engine      # 127.0.0.1:4321, token "dev", demo data in .dev-data/
npm run dev
```

with a `.env.development.local` containing:

```sh
VITE_ENGINE_URL=http://127.0.0.1:4321
VITE_ENGINE_TOKEN=dev
```

`npm run tauri dev` doesn't rebuild the engine when Go files change; run `npm run build:engine`.

## Test

```sh
npm test                # frontend (Vitest)
npm run test:engine     # Go engine
npm run check           # svelte-check / TypeScript
```

## How it fits together

Three processes:

1. **UI** (`src/`): a static SvelteKit SPA (Svelte 5, Tailwind v4).
2. **Shell** (`src-tauri/`): a thin Tauri v2 app that starts the engine and tells the UI how to reach it.
3. **Engine** (`engine/`): a Go sidecar that owns all mail logic: IMAP sync, SMTP sending, storage and search.

The UI talks to the engine's HTTP API on `127.0.0.1`. Every request carries a per-launch token, and only the app's own origin is allowed.

## License

[MIT](LICENSE)
