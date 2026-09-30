# Telegram user-account client

PRobot can manage Telegram **user accounts over MTProto**, using `gotd/td v0.162.0`
and `gotd/contrib v0.25.0`. The existing Telegram Bot API publisher remains a separate
integration. This subsystem runs inside `cmd/api`; it does not add a daemon or Python
process, and it is disabled by default.

## Repository integration

The existing project uses chi handlers in `internal/http`, manually constructed
services in `internal/service`, domain DTOs in `internal/domain`, pgx repositories
in `internal/store`, goose SQL migrations and Asynq workers. Configuration comes
from environment variables and optional `.env`; logging uses the standard `log`
package. HTTP authentication uses JWT plus workspace membership. There was no event
bus, WebSocket/SSE transport or metrics exporter. Existing tests are Go unit tests;
there are no Makefile test/lint targets or golangci-lint configuration.

The new dependency path is:

```text
chi/JWT/workspace-owner middleware
  -> service.TelegramService (workspace ownership checks)
    -> telegram.AccountManager (account lifecycle and operation cancellation)
      -> telegram.Client interface
        -> gotd adapter, peer resolver, auth state machine and updates manager
```

`cmd/api` constructs these dependencies explicitly. Telegram entities remain in
`internal/telegram`; HTTP and service methods use application DTOs. `TelegramStore`
uses the existing PostgreSQL pool. Each account lease uses one additional dedicated
PostgreSQL connection. No new monitoring stack or mock-generation framework is added.

## Configuration and rollout

Apply migration `00008_telegram_client.sql` using the existing goose/Compose flow.
Then set these through the deployment's secret configuration:

```dotenv
TELEGRAM_CLIENT_ENABLED=true
TELEGRAM_APP_ID=<application-id>
TELEGRAM_APP_HASH=<application-hash>
TELEGRAM_RPC_RATE=1
TELEGRAM_MAX_ACCOUNTS=10
ENCRYPTION_KEY=<private-32-byte-key>
```

Use credentials for your Telegram application, not a bot token. A public/default
`ENCRYPTION_KEY` is rejected when the subsystem is enabled. Keep the existing private
key if PRobot already has encrypted channel credentials. Both Compose variants pass
the new settings to the application. No filesystem volume is needed for sessions.
Disable the feature before rolling back its migration.

The rate is per account, with burst 1 (allowed configuration: greater than 0, at most
10 RPC/s). The account runtime limit defaults to 10; allow enough PostgreSQL connections
for the application pool plus one lease connection per active account. Login operations
expire after five minutes. APIs have a 45-second request timeout; no request waits for
human input. A QR-start request waits only for the first token to be exported.

Deploy with **one API instance owning Telegram runtimes**, or route all Telegram
operations to that instance. PostgreSQL advisory locks prevent duplicate account
sessions if instances overlap during deployment, but do not implement distributed
routing, automatic ownership handoff or high availability. A lease connection is checked
every five seconds with a three-second timeout; loss cancels that account runtime.

## Lifecycle

1. Create account metadata within a workspace. It is initially disabled/stopped.
2. Start reserves one runtime, enables startup restoration and acquires the account lease.
3. The runtime loads the encrypted session and starts gotd under an application child context.
4. With no authorization it becomes `auth_required`; after login it calls `Self()`, saves
   account identity and initializes the persistent updates manager.
5. `connected` means the updates manager has initialized; startup difference recovery
   and live updates then run under gotd. Recovery may still be catching up.
6. Stop disables restoration, cancels the runtime, waits for it and releases the lease.
   It preserves the session. Logout additionally revokes authorization and deletes
   session/peer/update state after the runtime stops.
7. On application shutdown the root context is cancelled, HTTP requests/SSE streams
   exit, and the manager waits for account goroutines before the PostgreSQL pool closes.

Repeated starts/stops are idempotent while already running/stopped. A start during
shutdown of the same runtime reports a conflict. Account errors are isolated; failed
accounts can be started again without restarting the API. Network reconnection,
keepalive and DC migration use gotd, with its unbounded reconnect backoff rather than
an application reconnect loop. Invalid/revoked Telegram sessions become `auth_required`
and are discarded after the runtime exits. Storage/decryption failures preserve data
and fail the account; fix the key/database before restarting it.

## Storage and secret handling

Migration 00008 creates:

- `telegram_accounts`: workspace, enabled flag, name, Telegram identity, phone,
  username, status and connection timestamps. No password, login code or API secret.
- `telegram_state`: per-account encrypted session, peer access hashes/entities,
  username cache and update checkpoints (`pts`, `qts`, `date`, `seq`, per-channel pts
  and access hashes). gotd entity caches use TL serialization inside encrypted values.
- `telegram_events`: deduplicated durable message events with encrypted payloads;
  metadata contains account ID, event type, chat/message IDs and timestamps.

Values use versioned AES-256-GCM with random nonces and storage-location AAD, using
`ENCRYPTION_KEY`. Moving ciphertext to another account/key fails authentication.
Back up the key separately from the database. Key rotation/re-encryption tooling is
outside this PR; changing the key without migrating encrypted values makes them unreadable.

Codes, 2FA passwords, API hash, auth keys and QR tokens are not logged or persisted in
plain text. 2FA uses gotd's SRP password helper and clears its temporary byte buffer;
Go/HTTP strings cannot guarantee cryptographic memory erasure. Login phone/code hash
live only in memory until success, expiry or shutdown. QR URLs exist only in the
in-memory operation and authenticated auth responses (the intentional exception
needed to display a QR). Responses have `Cache-Control: no-store`.

Telegram routes use a log format with route templates rather than actual URLs or
queries. RPC errors retain their underlying cause for `errors.Is/As`, but their public
message and HTTP mapping omit raw RPC details. Library logging uses its no-op default;
application logs contain account/operation/status/wait duration, not message content.

## REST API

All routes are under:

```text
/api/v1/workspaces/{workspaceID}/telegram
```

Use `Authorization: Bearer <PRobot JWT>`. **Workspace owner** access is required,
including read access to private dialogs/history/events. Every service method checks
that the target account belongs to that workspace before using its runtime. Do not
put credentials in URL/query parameters.

| Method | Path | Body or query |
|---|---|---|
| GET | `/accounts` | List metadata/status |
| POST | `/accounts` | `{"name":"Personal account"}` |
| GET | `/accounts/{id}` | Metadata/status |
| POST | `/accounts/{id}/start` | Start/restore runtime |
| POST | `/accounts/{id}/stop` | Stop; retain session |
| POST | `/accounts/{id}/logout` | Revoke and remove local state |
| GET | `/accounts/{id}/auth` | Current login state/refreshed QR URL |
| POST | `/accounts/{id}/auth/qr` | No body required |
| POST | `/accounts/{id}/auth/phone` | `{"phone":"+15551234567"}` |
| POST | `/accounts/{id}/auth/code` | `{"login_id":"...","code":"..."}` |
| POST | `/accounts/{id}/auth/password` | `{"login_id":"...","password":"..."}` |
| GET | `/accounts/{id}/dialogs` | `limit=1..100`, optional opaque `cursor` |
| GET | `/accounts/{id}/peers/{peer}` | Resolve peer to application DTO |
| GET | `/accounts/{id}/chats/{peer}/messages` | `limit=1..100`, `offset_id` |
| POST | `/accounts/{id}/chats/{peer}/messages` | `{"text":"Hello"}` |
| PATCH | `/accounts/{id}/chats/{peer}/messages/{messageID}` | `{"text":"Edited"}` |
| DELETE | `/accounts/{id}/chats/{peer}/messages/{messageID}` | Optional `revoke=true` |
| POST | `/accounts/{id}/chats/{peer}/messages/{messageID}/forward` | `{"to":"@username"}` |
| GET | `/accounts/{id}/events` | `after=eventID`, `limit=1..100` |
| GET | `/accounts/{id}/events/stream` | SSE; `Last-Event-ID` header or `after` |

Peers accept `@username`, bare username, `me`, or numeric **TDLib-style IDs**:
positive user ID, negative group ID, `-1000000000000-channelID` for channels/supergroups.
Load dialogs first to discover numeric peers/access hashes. Username resolution uses
gotd's LRU resolver plus an account-scoped persistent five-minute alias cache; it does
not resolve usernames before every send. Domain DTOs expose the same numeric IDs.

History returns an array, newest first; use the oldest returned message's ID as the
next `offset_id`. Service messages retain IDs/dates and have `service: true`, with no
Telegram action object exposed. Dialog responses have `items` and an opaque
`next_cursor`. Pages are bounded at 100; the implementation does not crawl full history.

### QR login

Start the account and poll its status until `auth_required`. POST `/auth/qr` returns
`login_id`, `state`, `url: "tg://login?token=..."`, operation `expires_at`, and
`token_expires_at`. Render that URL with a frontend QR library. Scan it from Telegram's
Devices screen. Poll GET `/auth` to refresh the displayed token and see `authorized`
or `waiting_password`. QR tokens rotate automatically within the operation TTL.
If 2FA is needed, submit the password with that same `login_id`.

### Phone/code/2FA

POST `/auth/phone` returns `waiting_code` and `login_id`. POST `/auth/code` advances to
`authorized` or `waiting_password`; submit `/auth/password` only in the latter state.
Wrong codes/passwords produce safe errors and can be retried. Stale login IDs or
concurrent login attempts return 409. Expired operations must be restarted. Account
registration/sign-up is not supported; use an existing Telegram account. Delivery
methods are controlled by Telegram and may require using the official app or QR.

## Updates, recovery and events

A `tg.NewUpdateDispatcher` normalizes new ordinary/channel messages, including service
messages, into application events. The handler performs only normalization and a
bounded SQL inbox write. It does not run business subscribers inside a Telegram update
goroutine. Subscribers consume the durable journal by cursor or use the SSE API.

`telegram/updates` restores account and channel checkpoints and performs difference
recovery on startup. `UpdateHook` captures RPC-returned updates; `AffectedHook` keeps
local pts synchronized after self-initiated reads/deletes. Observed peer entities,
channel hashes and user hashes are persisted separately for each account.

The event key `(account_id, type, chat_id, message_id)` deduplicates replay. An account's
event transactions serialize before allocating cursor IDs, so readers do not skip a
late commit. If event/state persistence fails, checkpoint writes are frozen and the
account is cancelled; restarting replays from the last durable checkpoint. This is
at-least-once recovery with a deduplicated inbox, not a distributed exactly-once promise.
Telegram may return `differenceTooLong` for sufficiently old/large gaps: a safe
`history_resync_required` log is emitted; automatic full-history backfill is not included.

Event types are `telegram.message.received` and `telegram.message.sent`. Example:

```json
{"id":123,"type":"telegram.message.received","account_id":"...","payload":{"id":7,"chat_id":42,"sender_id":42,"text":"Hello","outgoing":false}}
```

SSE reads the journal every second, writes heartbeats, disables proxy buffering, and
supports replay with `Last-Event-ID`. Streams close after one minute so reconnects
recheck JWT/membership. Use streaming `fetch` (or an SSE client supporting Authorization
headers); native browser `EventSource` cannot supply the bearer header. No token-in-URL
fallback is provided. Event retention/cleanup policy is deliberately not imposed by
this PR; provision storage and add a product retention policy before large deployments.

## Flood wait and files

Each client runs the official floodwait scheduler around gotd and applies a rate limiter
to RPC attempts. Flood waits sleep with cancellable contexts; a single wait is capped
at one minute with at most three retries. Account status exposes `flood_wait` while a
reported wait is active. Excess waits become a safe HTTP 429. Message content and
RPC arguments are not logged. No application-level resend loop is added: a timeout
can be ambiguous, so callers should check history before manually retrying a send.

`TelegramService.SendDocument`, `SendPhoto` and `Download` provide the file foundation.
Uploads use gotd uploader with one worker and bounded chunks; downloads resolve a
message attachment and stream via gotd downloader/media DC. Readers/writers are
provided by the caller, and must honor cancellation when their I/O can block. The
first version limits uploads to 2 GiB and does not add file HTTP endpoints, album/gallery
support, attachment indexing or automatic file-reference-refresh retries.

## Tests

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/...
```

Unit tests use small fakes without Telegram credentials. They cover lifecycle,
duplicate starts/stops, concurrent sends, multiple accounts/failure isolation,
invalid-session handling, shutdown/cancellation, session encryption and account AAD,
peer/update persistence, auth state transitions/TTL/QR cancellation, message mapping,
actual gotd gap recovery with a fake RPC API, checkpoint freeze after inbox failure,
workspace ownership, request limits, safe errors and log redaction.

Optional PostgreSQL integration test (never point this at production):

```sh
TEST_TELEGRAM_DATABASE_URL='postgres://test:test@127.0.0.1:5432/test?sslmode=disable' \
  go test -race ./internal/store -run TestTelegramPostgresIntegration -v
```

It creates a uniquely named schema, applies repository migrations, checks sessions,
exclusive leases, event deduplication/cursors/encryption and migration 00008 down/up,
then removes only that schema. No environment file is loaded by this test. The test
skips unless its explicit test database variable is present.

## Manual acceptance test (dedicated test accounts)

1. Migrate, enable the feature with test application credentials/private encryption key,
   start API, and sign in to PRobot as a workspace owner.
2. Create account A, start it and complete QR login (including 2FA if enabled).
3. Create account B, start it and complete phone/code/2FA. Verify independent identities.
4. List dialogs, follow `next_cursor`; fetch two bounded history pages with `offset_id`.
5. Send a message to `me` or a consenting test contact. Edit, forward and delete that
   test message. Confirm received/sent application events and SSE cursor replay.
6. Stop the backend, send a new message to A from the consenting test account, and
   restart it. Confirm automatic authorization restoration and recovery of that message
   exactly once in the durable event inbox. B must remain independent.
7. Stop A: it must not restart after a backend restart. Start A: no new login is needed.
8. Revoke A's session in Telegram. Verify `auth_required`; B remains operational.
   Start A again and authorize. Logout A and verify a new login is required on next start.
9. Cancel/shutdown while a QR operation is pending. Confirm clean exit and no retained
   QR operation. Restart and perform a fresh login.
10. Try another workspace, a viewer/editor JWT, and an expired JWT: all account operations
    and SSE must be denied. Inspect logs for absence of codes/passwords/QR URLs/content.
11. Exercise temporary network loss and reconnect. Observe naturally occurring flood
    waits or a fake RPC in tests; do not deliberately spam Telegram to trigger limits.

Live Telegram acceptance requires these manual steps; unit tests do not prove that
Telegram will accept a particular application's credentials or login delivery method.

## Follow-up scope

A subsequent PR can add frontend QR/auth screens and chat UI, a retention policy,
file transport endpoints/reference refresh, resumable history backfill for too-long
gaps, and explicit runtime ownership/routing for multiple API replicas. Calls, stories,
secret chats, payments and account-wide contact synchronization are out of scope.
