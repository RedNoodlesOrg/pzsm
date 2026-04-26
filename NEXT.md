# pzsm -- rewrite handoff

Go port of the old Python app at `../rednet-app-pzsm`.
Stack: Go 1.24, SQLite (`modernc.org/sqlite`). HTTP surface is JSON-only at `/api/*` (plus `/healthz`); the Mantine SPA in the sibling `../pzsmui` repo is the only frontend.

Behind Cloudflare Access in prod; local auth bypass via `dev_user_email` in the YAML config is gated behind the `devbypass` build tag and is dead-code-eliminated from the default (prod) build.

## Done

- **Slice 1 -- foundation.** YAML-driven config; SQLite store with embedded migrations; CF Access middleware (email from `Cf-Access-Authenticated-User-Email`, rejects unauth with 401); activity logger writing to both `log/slog` and the DB; structured request logging per authed route; graceful shutdown; distroless Dockerfile.
- **Slice 2 -- Steam.** Steam client (`GetCollectionDetails`, `GetPublishedFileDetails`, recursive `ExpandCollection`); `mods.Service` with `Sync` / `List` / `Toggle` (atomic via `RETURNING`). `Sync` reconciles both directions: ids in the collection are upserted, rows whose workshop_id is no longer in the expansion are deleted (cascading `mod_ids`); skipped if expansion returns zero ids so a transient empty response can't wipe the table.
- **Modern Steam API for file details.** `GetPublishedFileDetails` switched from the legacy `ISteamRemoteStorage` endpoint to `IPublishedFileService/GetDetails/v1/` (GET, key required). The legacy endpoint masked unlisted / non-default-visibility items as `result=9`; the modern one returns them. Two real mods in the live collection (`2906936402` Fluffy Hair, `2688809268` TCCacheMP) were previously silently dropped and now sync correctly. `description` json tag now reads `file_description`. Collection expansion still uses the legacy endpoint -- no key needed and it works.
- **Slice 3 -- `servertest.ini` writer.** `internal/serverini.UpdateMods(path, enabledModIDs, workshopIDs)` rewrites just the first `Mods=` and `WorkshopItems=` lines with `;`-joined values; every other line preserved byte-for-byte including CRLF; atomic write via temp file + rename, original mode preserved. Config key: `servertest_ini`, defaults to `{pz_server_folder}/Server/servertest.ini`. Seven serverini tests cover round-trip, CRLF preservation, empty lists, missing-line errors, leading whitespace, and missing file.
- **Auth bypass gated by build tag.** `dev_user_email` fallback lives behind `//go:build devbypass`; default builds constant-fold the branch out via `middleware.DevBypassEnabled = false`. Dockerfile builds without the tag.
- **Review-driven cleanup.** Dropped `description` column from `mods` (dead weight); `activity.Record` returns nothing and logs audit failures internally; `PublishedFileDetails.OK()` replaces the magic `Result == 1`; `ExtractModIDs` now returns `[]string` and the "single id auto-enables" policy lives in `mods.Service.Sync` rather than in the parser.
- **Load order.** Ordering persisted in `mods.position` (migration `0002_mod_position.sql`). `Service.ListByPosition`, `Reorder` (bulk validation + dense reindex, disabled mods appended in their existing order), and `MoveTo` (single-item insert with clamp). `mods/apply` reads from `ListByPosition` so `Mods=` emission follows the user-chosen order. Ten `mods` service tests cover reorder, move, validation, and position listing.
- **JSON API surface (`internal/api/`).** `GET /api/mods`, `GET /api/mods/ordered`, `POST /api/mods/{ws}/ids/{mid}/toggle`, `POST /api/mods/apply`, `POST /api/mods/reorder`, `POST /api/mods/{ws}/move`. DTO-layered JSON (`modDTO` in `dto.go`) decouples wire shape from `mods.Mod`. CF Access middleware applies, mounted at `/api/`.
- **SSE sync progress (`GET /api/mods/sync`).** `mods.Service.Sync` takes a non-blocking `progress chan<- SyncEvent`; the SSE handler emits `event: progress` frames for `expanding` / `fetching` / `upserting` stages and a final `event: done` with `SyncResult`, or `event: error` on failure. `middleware.statusRecorder` forwards `Flush` so streaming works through the logging wrapper.
- **Mod-id extractor robustness.** Regex anchored to start of line; tolerates `[b]Mod ID:[/b] X`, `Mod ID (b41): X`, and strips trailing/inline BBCode from the captured value. Catches `OLD MOD ID:` and plural `Mod IDs:` headings. 14 `TestExtractModIDs` subtests.
- **Fixture-backed tests.** Live responses for collection `3707778024` captured in `internal/steam/testdata/` (248 ids, all OK via the modern endpoint, no sub-collections). Five steam tests cover expand, fetch, empty-input short-circuit, missing-API-key error, and the modid extractor.
- **Legacy HTMX UI removed.** `internal/server/` and `internal/web/` deleted; the Go binary is API-only and the SPA in `../pzsmui` is the only frontend.
- **Servertest.ini editor.** `internal/serverini` gained `Read` (entries with attached `#` comments, blank-line breaks attachment) and `WriteFields` (atomic partial rewrite, only the first occurrence of each key is rewritten, CRLF preserved, errors on missing keys); `UpdateMods` now wraps `WriteFields`. New `GET`/`PUT /api/serverini` (`internal/api/serverini.go`) with three policy buckets enforced server-side: **hidden** (`Mods`, `WorkshopItems`, `Map` — filtered out of GET, rejected by PUT), **readonly** (`ResetID`, `ServerPlayerID`, `DefaultPort`, `UDPPort`, `RCONPort` — returned but PUT rejects), **secret** (`RCONPassword`, `Password`, `DiscordToken` — value blanked on GET, empty value on PUT is a no-op). PUT logs a `serverini.update` activity row. SPA `/config` page in `../pzsmui` renders a filterable flat list, infers input type per field (bool→Switch, `Minimum=…Maximum=…`→NumberInput w/ bounds, secret→PasswordInput, else→TextInput), only sends dirty fields. 17 new tests across the two backend packages.

## Next, in order

### SPA hosting (small)

The SPA at `../pzsmui` is currently dev-only (Vite proxies `/api` to `:8080`). To ship: build the SPA, embed `dist/` into Go via `go:embed` (new `internal/spa/` package?), serve assets at `/` with a fallback to `index.html` for client-side routes, add a node build stage to the Dockerfile.

### Slice 4 -- RCON (medium)

Replace docker-restart with `gorcon/rcon` (Source RCON protocol, same PZ uses). Handler `POST /api/cmd/restart` sends `quit`; PZ wrapper script restarts the process. Optional: `servermsg` broadcast before the quit so players get a warning. Keep docker-restart as fallback only if RCON fails to connect.

**Before writing Go code,** confirm PZ's RCON is actually reachable -- set `RCONPort`/`RCONPassword` in `servertest.ini`, verify the port is bound. The Python version's RCON "wasn't working" was very likely this rather than the code.

### Slice 5 -- game log parser for `/logs` (biggest)

PZ writes to `{pz_server_folder}/Zomboid/Logs/` -- plain text, multiple files (`*_user.txt`, `*_PerkLog.txt`, `*_pvp.txt`, `*_chat.txt`). Use `fsnotify` on the dir, tail active files, parse into a new `game_events` table (`id, kind, player, details_json, created_at`). Surface via a new read endpoint that the SPA's Logs page consumes -- SSE for live updates.

Decide which event kinds to capture *before* writing parsers; easy to over-build this.

## Run

```sh
cp config.example.yaml config.yaml
# edit config.yaml: at minimum set database_path, steam_collection_id,
# steam_web_api_key (https://steamcommunity.com/dev/apikey), dev_user_email
make run
```

Backend listens on `:8080` and serves `/api/*` and `/healthz`. For the UI, run the SPA dev server in `../pzsmui` (`yarn dev`); Vite proxies `/api` to the Go binary.

The `-tags devbypass` flag (set by `make run` and `make dev`) is required for `dev_user_email` to take effect; without it every request returns 401.

Prod: `make build` (no tags), run behind `cloudflared` with an Access policy that injects `Cf-Access-Authenticated-User-Email`. `dev_user_email` has no effect on a default build. The Dockerfile entrypoint expects the config at `/etc/pzsm/config.yaml` -- mount it.

## Test

```sh
go test ./...
```

Steam fixtures are real responses for collection `3707778024`. Regenerate if the collection structure changes meaningfully (e.g., sub-collections get added -- current fixture has none).

## Gaps / deferred

- No integration test for `mods.Service.Sync` end-to-end against in-memory SQLite. Client is covered; the upsert loop is not. `pruneMissing` is tested directly. The SSE handler in `internal/api/sync.go` has no test either.
- `ExtractModIDs` still captures trailing prose punctuation when a creator writes `Mod ID: X)` mid-sentence at line start. Not seen in the current `3707778024` fixture; the prior `3632610172` (TrueMoozic) case is fixed for the inline-prose-mention shape but `Mod ID: X)` at line start would still keep `X)`.
- `serverini.UpdateMods` has no end-to-end handler test; the package-level tests cover the file-rewrite logic only.
- Per-workshop ordering only -- mod ids within a single workshop still sort by `(length, mod_id)`. Revisit if any mod publishes multiple conflict-relevant ids.
- SPA is not embedded yet; production deployments need an out-of-band way to serve `pzsmui/dist/`.

## Orientation

| Path | Purpose |
|---|---|
| `cmd/pzsm/main.go` | entry point; wires config, store, steam, mods, api, middleware, graceful shutdown |
| `internal/config/config.go` | YAML config loader (`--config` flag, default `./config.yaml`); `servertest_ini` derived from `pz_server_folder` when omitted |
| `internal/store/store.go` + `migrations/` | SQLite + embedded migrations with `schema_migrations` bookkeeping |
| `internal/steam/` | API client, regex extractor, fixtures, tests |
| `internal/mods/mods.go` | `Service.Sync(ctx, id, progress)` / `List` / `ListByPosition` / `Toggle` / `Reorder` / `MoveTo`; `SyncEvent` on progress channel |
| `internal/serverini/` | `Read` (entries + attached comments) and `WriteFields` (atomic partial rewrite); `UpdateMods` is a thin wrapper |
| `internal/api/` | JSON + SSE surface at `/api/*` for the SPA; DTOs decouple wire shape from service. `serverini.go` owns the GET/PUT handlers and field-policy buckets |
| `internal/middleware/cfaccess.go` | identity injection + per-request slog line; `devbypass` tag gates `dev_user_email` fallback |
| `internal/activity/activity.go` | audit log to DB and slog |
| `internal/identity/identity.go` | context key for user email |

The old Python repo at `../rednet-app-pzsm` is the reference implementation. Don't delete until slice 5 is shipped and the old instance is retired.
