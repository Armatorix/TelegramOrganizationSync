# TelegramOrganizationSync — Design Specification

## 1. Overview

A long-running Go daemon that uses [TDLib](https://core.telegram.org/tdlib/docs/)
to act as a Telegram client (logged in as a regular user account or bot with
admin rights) and reconciles the membership of one or more Telegram channels
against an authoritative organization server.

The client periodically:

1. Asks the server which channels are under sync and who manages them.
2. For each managed channel, snapshots the current Telegram member list and
   ships it to the server.
3. Receives a diff (`add`, `remove`) and either:
   - **auto mode** — executes the diff on Telegram, or
   - **manual mode** — DMs the channel manager with the proposed diff.
4. Discovers new Telegram channels the account is in and registers them on
   the server with `sync_status = disabled` so a human can opt them in.

## 2. Goals & Non-goals

**Goals**
- Single, stateless-ish binary; all source-of-truth state lives on the server.
- Two-way channel registry: the server knows about a channel either because a
  human added it there, or because the client discovered it on Telegram.
- Safe-by-default: new channels start disabled; manual mode never mutates.
- Bounded API surface — three server endpoints, easy to mock and test.

**Non-goals (v1)**
- Message moderation, content sync, media handling.
- Multi-tenant / multi-organization support in a single client process.
- Direct database access — the client only talks to the server's HTTP API.
- High-frequency real-time sync (TDLib updates are observed but reconciliation
  runs on a poll interval).

## 3. High-level architecture

```
            +--------------------+      HTTPS + API key       +------------------+
            |  TDLib (C lib)     |                            |                  |
   Telegram |  via JSON client   |  <----  Sync Engine  ---->  | Organization    |
   network  |  (cgo bindings)    |                            | Server (HTTP)    |
            +---------^----------+                            +------------------+
                      |
                      | tdlib events
                      v
            +--------------------+
            |  Telegram Adapter  |
            |  (members, kicks,  |
            |   invites, DMs)    |
            +--------------------+
```

Three internal layers:

- **Telegram Adapter** — thin wrapper around TDLib. Owns the C client handle,
  the auth state machine, and exposes a small Go interface
  (`ListMembers`, `AddMember`, `RemoveMember`, `SendDM`, `OnNewChannel`).
- **Server Client** — typed HTTP client for the three server endpoints.
- **Sync Engine** — orchestrator. Runs the reconciliation loop, owns the
  mode (`auto`/`manual`), translates between Telegram and server identifiers.

## 4. Configuration

Loaded from a YAML/TOML file and/or env vars (env wins for secrets).

```go
type Config struct {
    Server struct {
        URL    string `yaml:"url"     env:"TOS_SERVER_URL"`
        APIKey string `yaml:"api_key" env:"TOS_SERVER_API_KEY"`
    } `yaml:"server"`

    Mode Mode `yaml:"mode" env:"TOS_MODE"` // "auto" | "manual"

    Telegram struct {
        APIID       int32  `yaml:"api_id"       env:"TOS_TG_API_ID"`
        APIHash     string `yaml:"api_hash"     env:"TOS_TG_API_HASH"`
        DatabaseDir string `yaml:"database_dir" env:"TOS_TG_DB_DIR"`
        // PhoneNumber for user accounts, BotToken for bots — exactly one.
        PhoneNumber string `yaml:"phone_number" env:"TOS_TG_PHONE"`
        BotToken    string `yaml:"bot_token"    env:"TOS_TG_BOT_TOKEN"`
    } `yaml:"telegram"`

    Sync struct {
        Interval     time.Duration `yaml:"interval"      env:"TOS_SYNC_INTERVAL"`      // e.g. 5m
        BatchSize    int           `yaml:"batch_size"    env:"TOS_SYNC_BATCH"`         // members per page
        DryRun       bool          `yaml:"dry_run"       env:"TOS_DRY_RUN"`
    } `yaml:"sync"`
}

type Mode string
const (
    ModeAuto   Mode = "auto"
    ModeManual Mode = "manual"
)
```

The three setup fields the user called out (`server url`, `server api key`,
`mode`) are the only ones strictly required; Telegram credentials are needed
to bring up TDLib but are operational, not policy.

## 5. Server API contract

All requests:
- `Authorization: Bearer <api-key>` header.
- JSON in / JSON out, UTF-8.
- Errors use RFC 7807 `application/problem+json`.

### 5.1 `GET /api/v1/channels`

Lists every channel the server knows about and whether the client should
reconcile it.

```json
[
  {
    "id": "ch_01HX...",
    "telegram_chat_id": -1001234567890,
    "title": "engineering-announce",
    "sync_status": "enabled",
    "manager": {
      "telegram_user_id": 42424242,
      "name": "Anna Manager"
    }
  }
]
```

`sync_status`: `"enabled" | "disabled"`. Only `enabled` rows are reconciled.
`manager.telegram_user_id` is required so the client can DM in manual mode.

### 5.2 `POST /api/v1/channels`

Called by the client when it discovers a Telegram channel that the server
does not yet know about. Idempotent on `telegram_chat_id`.

Request:
```json
{
  "telegram_chat_id": -1001234567890,
  "title": "engineering-announce"
}
```

Response: the full channel record, with `sync_status: "disabled"` enforced by
the server. The client never gets to enable sync — that is a human decision
made on the server side.

### 5.3 `POST /api/v1/channels/{id}/members:reconcile`

Snapshot-and-diff endpoint. The client sends what Telegram currently shows,
the server replies with what to change.

Request:
```json
{
  "members": [
    { "telegram_user_id": 11111, "name": "Alice" },
    { "telegram_user_id": 22222, "name": "Bob"   }
  ]
}
```

Response:
```json
{
  "to_add":    [ { "telegram_user_id": 33333, "name": "Carol" } ],
  "to_remove": [ { "telegram_user_id": 22222, "name": "Bob"   } ]
}
```

The client treats `to_add`/`to_remove` as authoritative for this tick and
does not re-derive them locally.

## 6. Sync engine

```go
type Engine struct {
    cfg    Config
    tg     TelegramAdapter
    server ServerClient
    log    *slog.Logger
}

func (e *Engine) Run(ctx context.Context) error {
    // 1. Bring up TDLib, block until authorized.
    // 2. Subscribe to "new chat" updates -> registerDiscoveredChannel.
    // 3. Tick on cfg.Sync.Interval -> reconcileAll.
}
```

### 6.1 Tick loop

```
for each tick:
    channels := server.ListChannels()
    for ch in channels where sync_status == enabled:
        members := tg.ListMembers(ch.telegram_chat_id)
        diff    := server.Reconcile(ch.id, members)
        applyDiff(ch, diff)
```

### 6.2 `applyDiff` — mode dispatch

```go
func (e *Engine) applyDiff(ch Channel, diff Diff) error {
    if len(diff.ToAdd) == 0 && len(diff.ToRemove) == 0 {
        return nil
    }
    switch e.cfg.Mode {
    case ModeAuto:
        return e.applyAuto(ch, diff)
    case ModeManual:
        return e.notifyManager(ch, diff)
    }
}
```

- **Auto** — call `tg.AddMember` / `tg.RemoveMember` per entry, with
  per-call error capture; one failure does not abort the rest of the diff.
  Emit a structured log line per action so the action is auditable.
- **Manual** — render a single message to `ch.Manager.TelegramUserID`
  listing the proposed adds/removes and the channel name. Never mutate
  membership.

### 6.3 New-channel discovery

TDLib emits `updateNewChat` and similar events. The adapter translates these
into a `<-chan DiscoveredChannel` that the engine consumes and forwards to
`POST /channels`. The server's idempotency on `telegram_chat_id` means
re-emitted events are harmless.

## 7. Telegram adapter

TDLib is a C library; the adapter hides cgo. Recommended binding:
[`github.com/zelenin/go-tdlib`](https://github.com/zelenin/go-tdlib) (active,
JSON-client based) — wrap it behind our own interface so we can swap or fake
it in tests.

```go
type TelegramAdapter interface {
    ListMembers(ctx context.Context, chatID int64) ([]Member, error)
    AddMember(ctx context.Context, chatID, userID int64) error
    RemoveMember(ctx context.Context, chatID, userID int64) error
    SendDM(ctx context.Context, userID int64, text string) error
    DiscoveredChannels() <-chan DiscoveredChannel
}

type Member struct {
    TelegramUserID int64
    Name           string
}
```

Notes specific to TDLib:
- Member listing uses `getSupergroupMembers` with paging up to the supergroup
  size limit; `getChatMember` is for spot-checks. The adapter pages
  internally and returns the full list.
- `AddMember` maps to `addChatMember`, which only succeeds if the target user
  has previously interacted with the bot/account or shares a contact;
  failures must be surfaced as typed errors so the engine can decide whether
  to fall back to manual notification on a per-user basis.
- `RemoveMember` maps to `setChatMemberStatus` with `chatMemberStatusLeft`.
- The TDLib working directory (`database_dir`) holds session keys — must be
  on persistent storage and **must not** be checked into git.

## 8. Project layout

```
.
├── cmd/
│   └── tos/                 # main; flags, config load, signal handling
├── internal/
│   ├── config/              # Config struct + loader (yaml + env)
│   ├── server/              # typed HTTP client for the 3 endpoints
│   ├── telegram/            # TDLib adapter + interface + fake for tests
│   ├── sync/                # Engine, tick loop, diff application
│   └── log/                 # slog setup
├── DESIGN.md                # this file
├── go.mod
└── go.sum
```

`internal/` keeps the surface area unimportable from outside, which matches
the "single binary" goal.

## 9. Reconciliation sequence (auto mode)

```
Engine            Server                Telegram (TDLib)
  |                  |                         |
  |-- GET channels ->|                         |
  |<--- list --------|                         |
  |                                            |
  | for each enabled channel:                  |
  |--------------- ListMembers --------------->|
  |<-------------- members ---------------------|
  |                                            |
  |- POST reconcile(members) -->|              |
  |<------- {to_add, to_remove} |              |
  |                                            |
  |--- AddMember(u) for each u in to_add ----->|
  |--- RemoveMember(u) for each u in to_remove>|
```

Manual mode replaces the last two arrows with a single `SendDM(manager, body)`.

## 10. Error handling & operational concerns

- **Server unavailable** — skip this tick, log a warning, retry next interval.
  Never act on stale data.
- **TDLib unauthorized** — the adapter exposes an auth-state channel; the
  engine refuses to start ticking until the state reaches `Ready`.
- **Partial diff failure** in auto mode — record per-user outcome; the next
  tick will re-derive and naturally retry, because the server diff is
  recomputed from a fresh snapshot each time. No local retry queue needed.
- **`dry_run`** — short-circuits both `applyAuto` and `notifyManager` to log
  only. Useful for first deployments.
- **Observability** — structured logs (`slog`) with `channel_id`,
  `telegram_chat_id`, `tick_id`. Optional Prometheus counters:
  `tos_tick_total`, `tos_diff_apply_total{action,result}`,
  `tos_server_request_duration_seconds`.

## 11. Security

- API key transits only over TLS; rejected at startup if `server.url` is not
  `https://` (escape hatch for local dev: explicit `--insecure` flag).
- The TDLib database directory is chmod 0700 and contains the only
  credential material the client cannot rotate by config change.
- Server responses are size-capped — a malicious or buggy server cannot make
  the client kick the entire channel by returning an absurd `to_remove` list:
  if `len(to_remove) > len(currentMembers) * 0.5`, the engine refuses to
  apply in auto mode and falls back to manager notification with a warning.

## 12. Open questions

1. Should manual-mode notifications include actionable buttons (TDLib inline
   keyboards) so the manager can approve directly in Telegram, or stay
   purely informational and rely on the server's UI? v1 assumes informational.
2. How should the client handle a channel transitioning from `enabled` →
   `disabled` mid-flight? v1: respect the status read at the start of the
   tick; do not abort an in-progress diff.
3. Bot vs. user account: bots can be admins of channels but cannot read the
   full member list of large supergroups via TDLib. If the deployment target
   has channels >200k members, a user account is required. v1 supports both
   via the `phone_number` / `bot_token` mutual-exclusion in config.
