# AGENTS.md

## Project Shape

This is a Wails v3 desktop app: a normal Go program hosts a native WebView, serves the frontend into it, and exposes selected Go functionality to JavaScript through generated bindings. `main.go` currently composes the app, window, service registration, emitted events, and embedded assets.

The updated Wails docs describe this in v2 terms like `wails.Run`, lifecycle callbacks, and `Bind`. In this repo, the same ideas show up through `application.New(...)`, registered `Services`, manager APIs on `app`, and generated files in `frontend/bindings/`.

## Core Concepts

### Manager API

- Prefer the organized managers on `app` such as `app.Window`, `app.Event`, `app.Dialog`, `app.Menu`, `app.Browser`, and `app.Env`.
- Use managers for native app concerns and keep business logic in services.
- This repo already uses the manager style in `main.go` via `app.Window.NewWithOptions(...)` and `app.Event.Emit(...)`.

### Lifecycle

- Treat services as the lifecycle unit. Put databases, config loaders, timers, and background workers in services with `ServiceStartup(ctx, options)` and `ServiceShutdown()`.
- Services start in registration order and shut down in reverse order. If `ServiceStartup` returns an error, app startup aborts cleanly.
- Use `ShouldQuit` for quit confirmation and `OnShutdown` for light app-level cleanup, not for long-lived resource management.
- Any goroutine that survives beyond a single call should respect `ctx.Done()` or have explicit cancellation.

### Go-Frontend Bridge

- Exported methods on registered services are scanned and turned into generated TypeScript bindings in `frontend/bindings/`, so they can be called from the frontend like local async functions.
- Change Go service APIs first and never hand-edit generated bindings.
- Keep bridge APIs simple and typed: primitives, slices, maps, structs, and `error` work well.
- Struct fields need usable JSON shapes to translate cleanly to TypeScript. Avoid anonymous nested structs and bridge-unfriendly types such as channels, funcs, file handles, or opaque interfaces.
- Batch related work when possible. Use events for progress or streaming instead of polling or returning huge payloads.

### Build System

- Wails build flow is: analyze Go services -> generate bindings -> build frontend -> compile Go -> embed `frontend/dist`.
- In development, assets are served from disk with live reload. In production, `frontend/dist` is embedded into the binary and no external frontend files are shipped.
- Use `task dev` for normal development. In this repo it runs `wails3 dev` through `build/config.yml` and the Taskfile-managed Vite port.
- Use `task build` for release-style validation: production frontend build, optimized Go binary, embedded assets.
- If you change service signatures, startup wiring, or asset loading, run a full build before finishing.
- Keep `frontend/dist` lean and make sure it still contains `index.html`, because embedded assets directly affect both startup and binary size.

## Repo Map

- `main.go`: app bootstrap, manager usage, window config, service registration, event emission, embedded asset wiring
- `greetservice.go`: example Go service exposed to the frontend
- `frontend/src`: Vue application source
- `frontend/bindings`: auto-generated Wails bridge code; treat as generated output
- `Taskfile.yml`: preferred project commands
- `build/config.yml`: Wails dev/build orchestration

## Working Notes

- Use `task dev` for day-to-day development.
- Use `task build` before finishing changes that affect startup, bindings, or assets.
- If you add or rename service methods, confirm the frontend bindings still regenerate cleanly through the normal Wails dev/build flow.
- Keep backend/frontend changes aligned: service API changes usually require matching TypeScript updates.
- Prefer moving persistent timers or workers out of `main.go` and into services so shutdown is coordinated by context.
