# scuttlebot Bootstrap

This is the primary conventions document. All agent shims (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `calliope.md`) point here.

An agent given this document and a business requirement should be able to generate correct, idiomatic code without exploring the codebase.

---

## What is scuttlebot

An agent coordination backplane built on IRC. Agents connect as IRC users, coordinate via channels, and communicate via structured messages. IRC is an implementation detail — users configure scuttlebot, never Ergo directly.

**Why IRC:** lightweight TCP transport, encryption, channels, presence, ops hierarchy, DMs, human observable by default. Humans and agents share the same backplane with no translation layer.

**Ergo** (https://ergo.chat) is the IRC server. scuttlebot manages its lifecycle and config. Federation, auth, history, TLS, rate limiting — all Ergo. scuttlebot abstracts it.

---

## Monorepo Layout

```
cmd/
  scuttlebot/     # daemon binary
  scuttlectl/     # CLI/REPL binary
internal/
  ergo/           # ergo lifecycle + config generation
  registry/       # agent registration + credential issuance
  topology/       # channel provisioning + mode/topic management
  bots/           # built-in bots (scribe, scroll, herald, oracle, warden)
  mcp/            # MCP server for AI agent connectivity
internal/config/  # config loading + validation
pkg/
  client/         # Go SDK (public)
  protocol/       # wire format (message envelope)
apps/
  web/            # operator UI — separate app, own stack
sdk/              # future: python, ruby, rust client SDKs
deploy/
  docker/         # Dockerfile(s)
  compose/        # docker compose (local dev + single-host)
  k8s/            # Kubernetes manifests
  standalone/     # single binary, no container required
go.mod
go.sum
```

Single Go module for everything under `cmd/`, `internal/`, `pkg/`. `apps/web/` and `sdk/*` are their own modules.

---

## Architecture

### Ergo relationship

scuttlebot owns the Ergo process and config. Users never edit `ircd.yaml` directly. scuttlebot generates it from its own config and manages Ergo as a subprocess.

- Ergo provides: TLS, SASL accounts, channel persistence, message history, ops hierarchy, server federation, rate limiting
- scuttlebot provides: agent registration, topology provisioning, rules-of-engagement delivery, built-in bots, SDK/MCP layer

### Agent lifecycle

1. Agent calls scuttlebot registration endpoint
2. scuttlebot creates Ergo account, issues SASL credentials
3. On connect, agent receives signed rules-of-engagement payload (channel assignments, engagement rules, permissions)
4. Agent connects to Ergo with SASL credentials
5. scuttlebot verifies presence, assigns channel modes

### Channel topology

Hierarchical, configurable. Convention:

```
#fleet                              fleet-wide, quiet, announcements only
#project.{name}                     project coordination
#project.{name}.{topic}             swarming, chatty, active work
#project.{name}.{topic}.{subtopic}  deep nesting
#task.{id}                          ephemeral, auto-created/destroyed
#agent.{name}                       agent-specific inbox
```

Users define topology in scuttlebot config. scuttlebot provisions the channels, sets modes and topics.

### Wire format

- **Agent messages:** JSON envelope in `PRIVMSG`
- **System/status:** `NOTICE` — human readable, machines ignore
- **Agent context packets** (summarization, history replay): TOON format (token-efficient for LLM consumption)

JSON envelope structure:

```json
{
  "v": 1,
  "type": "task.create",
  "id": "ulid",
  "from": "agent-nick",
  "ts": 1234567890,
  "payload": {}
}
```

### Authority / trust hierarchy

IRC ops model maps directly:
- `+o` (channel op) — orchestrator agents, privileged
- `+v` (voice) — trusted worker agents
- no mode — standard agents

### Built-in bots

| Bot | Role |
|-----|------|
| `scribe` | Structured logging to persistent store |
| `scroll` | History replay to PM on request (never floods channels) |
| `herald` | Alerts + notifications |
| `oracle` | Summarization — packages context as TOON for agent consumption |
| `warden` | Moderation + rate limiting |

v0 ships `scribe` only. Pattern proven, others follow.

### Scale

Target: 100s to low 1000s of agents on a private network. Single Ergo instance handles this comfortably (documented up to 10k clients, 2k per channel). Ergo scales up (multi-core), not out — no horizontal clustering today. Federation is planned upstream but has no timeline; not a scuttlebot concern for now.

### Persistence

| What | Standalone | Docker Compose / K8s |
|------|-----------|----------------------|
| Ergo state (accounts, channels, topics) | `ircd.db` local file | PersistentVolume (K8s) or named volume (Compose) |
| Ergo message history | in-memory buffer | MySQL (Ergo-native, unlimited history) |
| scuttlebot state (agent registry, config) | SQLite | Postgres |
| scribe bot (chat/event logs) | SQLite | Postgres or S3 |

K8s HA: single Ergo pod with PVC for `ircd.db`. Not multi-replica — Ergo is single-instance. HA = fast pod restart with durable storage.

---

## Conventions

### Go

- Go 1.22+
- `gofmt` + `golangci-lint`
- Errors returned, not panicked. Wrap with context: `fmt.Errorf("registry: create account: %w", err)`
- Interfaces defined at point of use, not in the package that implements them
- No global state. Dependencies injected via struct fields or constructor args.
- Config via struct + YAML/TOML — no env var spaghetti (env vars for secrets only)

### Tests

- `go test ./...`
- Integration tests use a real Ergo instance (Docker Compose in CI)
- Assert against observable state — channel membership, messages received, account existence
- Both happy path and error cases
- No mocking the IRC connection in integration tests

### Commits + branches

- Branch: `feature/{issue}-short-description` or `fix/{issue}-short-description`
- No rebases. New commits only.
- No AI attribution in commits.

---

## Adding a New Bot

1. Create `internal/bots/{name}/` package
2. Implement the `Bot` interface (defined in `internal/bots/bot.go`)
3. Register in `internal/bots/registry.go`
4. Add config struct to `internal/config/`
5. Write tests: bot handles valid message, ignores malformed message, handles disconnect/reconnect
6. Update this bootstrap

---

## Adding a New SDK

1. Create `sdk/{language}/` as its own module
2. Implement the client interface defined in `pkg/client/` as reference
3. Cover: connect, register, send message, receive message, disconnect
4. Own CI workflow in `.github/workflows/sdk-{language}.yml`

---

## Ports (local)

| Service | Address |
|---------|---------|
| Ergo IRC | `ircs://localhost:6697` |
| scuttlebot API | `http://localhost:8080` |
| MCP server | `http://localhost:8081` |

---

## Common Commands

```bash
go build ./cmd/scuttlebot      # build daemon
go build ./cmd/scuttlectl      # build CLI
go test ./...                  # run all tests
golangci-lint run              # lint
docker compose up              # boot ergo + scuttlebot locally
```
