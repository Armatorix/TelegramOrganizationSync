# TelegramOrganizationSync

A Go daemon that uses [TDLib](https://core.telegram.org/tdlib/docs/) to keep
Telegram channel membership in sync with an authoritative organization server.

The full design — endpoints, sequence flows, safety rails, open questions —
lives in [DESIGN.md](DESIGN.md). This README is the quickstart.

## Quickstart

```bash
make dev
```

That builds both binaries, copies example config files into place if missing,
and runs the dev server and the sync client side by side with prefixed output
(`[srv]` / `[cli]`). Ctrl-C stops both.

The dev server's admin UI is at <http://localhost:8080>. The default API key
is `dev-api-key`.

## Trying out a sync

The fake adapter ships two pre-populated channels in
`fake-telegram.example.json` — `engineering-announce` and `ops-incidents`.

1. Open <http://localhost:8080>. Both channels appear, auto-registered with
   `sync_status=disabled`.
2. On `engineering-announce`, set a manager (any `telegram_user_id` + name)
   and click **Enable sync**.
3. Paste an expected member list into the textarea (one per line:
   `<user_id> <name>`):
   ```
   100 Alice
   300 Carol
   ```
4. Wait for the next tick (default 30s) or restart `make dev`. The client
   computes the diff (`+Carol -Bob` against the fake state) and either:
   - **auto mode** — applies it directly; `fake-telegram.json` on disk is
     rewritten and the **Recent reconciliations** panel records the call;
   - **manual mode** — sends a single DM to the manager (logged in
     `fake-telegram.json` under `dms`).

Switch modes by editing `config.yaml`:

```yaml
mode: auto       # apply diffs directly via Telegram
# mode: manual   # only DM the manager with the proposed diff
```

## Make targets

| target           | what it does                                |
|------------------|---------------------------------------------|
| `make help`      | list all targets                            |
| `make build`     | compile both binaries to `./bin/`           |
| `make dev`       | run dev server + client together            |
| `make devserver` | run only the dev server                     |
| `make client`    | run only the sync client                    |
| `make setup`     | copy example config files into place        |
| `make vet`       | `go vet ./...`                              |
| `make tidy`      | `go mod tidy`                               |
| `make test`      | `go test ./...`                             |
| `make clean`     | remove `./bin/` (keeps your config files)   |

Override addresses or paths via env:

```bash
DEVSERVER_ADDR=:9000 DEV_API_KEY=secret CLIENT_CONFIG=other.yaml make dev
```

## Modes

- **auto** — the client calls `AddMember` / `RemoveMember` on Telegram for each
  entry in the diff. Safety rail: if a single reconcile asks to remove >50% of
  current members, auto mode refuses and falls back to a manager DM with a
  ⚠ prefix.
- **manual** — the client never mutates membership. It sends one DM to the
  channel manager listing the proposed adds and removes.

## Real Telegram (TDLib)

The default build uses a JSON-file-backed fake adapter, so `make dev` works
without TDLib installed. To talk to real Telegram:

1. Install the TDLib C library — see <https://tdlib.github.io/td/build.html>.
2. `go get github.com/zelenin/go-tdlib/client` and finish the wiring in
   [internal/telegram/tdlib.go](internal/telegram/tdlib.go) (the file is a
   scaffold with the right interface and a wiring checklist at the top).
3. Build with the `tdlib` tag: `go build -tags tdlib ./cmd/tos`.
4. In `config.yaml`, remove `telegram.fake_state_file` and fill in `api_id`,
   `api_hash`, `database_dir`, and exactly one of `phone_number` /
   `bot_token`.

## Configuration reference

All fields can also be supplied via env (`TOS_*`); env overrides the YAML.

| YAML path                  | env var                  | required when                | notes                                      |
|----------------------------|--------------------------|------------------------------|--------------------------------------------|
| `server.url`               | `TOS_SERVER_URL`         | always                       | base URL of the org server                 |
| `server.api_key`           | `TOS_SERVER_API_KEY`     | always                       | sent as `Authorization: Bearer <key>`      |
| `mode`                     | `TOS_MODE`               | always                       | `auto` or `manual`                         |
| `telegram.fake_state_file` | `TOS_TG_FAKE_STATE_FILE` | local dev                    | selects the fake adapter when set          |
| `telegram.api_id`          | `TOS_TG_API_ID`          | TDLib                        | Telegram API ID                            |
| `telegram.api_hash`        | `TOS_TG_API_HASH`        | TDLib                        | Telegram API hash                          |
| `telegram.database_dir`    | `TOS_TG_DB_DIR`          | TDLib                        | persistent dir for TDLib session keys      |
| `telegram.phone_number`    | `TOS_TG_PHONE`           | TDLib (user account)         | mutually exclusive with `bot_token`        |
| `telegram.bot_token`       | `TOS_TG_BOT_TOKEN`       | TDLib (bot)                  | mutually exclusive with `phone_number`     |
| `sync.interval`            | `TOS_SYNC_INTERVAL`      | optional                     | default `5m`                               |
| `sync.dry_run`             | `TOS_DRY_RUN`            | optional                     | log diffs without applying                 |

## Layout

```
cmd/tos/             # sync client
cmd/devserver/       # local org server with admin UI (dev only)
internal/api/        # wire types shared by client and server
internal/config/     # YAML + env config loader
internal/server/     # HTTP client for the three spec endpoints
internal/telegram/   # adapter interface, fake (default), TDLib (build-tagged)
internal/sync/       # tick loop, mode dispatch, safety rails
internal/devserver/  # in-memory store, admin handlers, embedded UI
DESIGN.md            # full design spec
```
