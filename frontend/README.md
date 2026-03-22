# Chisel Frontend

This frontend started from Nuxt UI's `ui/chat` template and has been adapted to fit this Wails desktop app.

## What Changed

- The app now runs as a static, client-side Nuxt build so Wails can embed `frontend/dist`.
- Chat history is stored locally in the browser/WebView instead of relying on the template's Nuxt server stack.
- The original auth, database, blob upload, and server-side AI integration paths were removed from the desktop build.

## Commands

Install dependencies:

```bash
pnpm install
```

Run the Nuxt dev server:

```bash
pnpm dev
```

Typecheck the frontend:

```bash
pnpm typecheck
```

Build the static frontend that Wails embeds:

```bash
pnpm build
```

## Notes

- `pnpm build` generates Nuxt output and copies the static assets into `frontend/dist`.
- The current chat replies are local placeholder responses so the UI works cleanly inside Wails.
- If you want real model responses later, the cleanest next step is wiring prompts to a Wails Go service or another backend API.
