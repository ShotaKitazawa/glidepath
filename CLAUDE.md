## Product scope

`SPEC.md` is the source of truth for product scope and feature priority
(B > A > D > C/E, see its section 1). Check it before adding a feature and
judge necessity against that priority order, not against statistical or
technical interestingness.

**Checking a change against "the purpose" means re-reading the relevant
SPEC.md prose (not just its schema) and checklisting it against the
implementation — not just checking internal consistency with your own
restated plan for the change.** A screen was once built straight from a
DB column list and missed a workflow requirement stated only in prose
elsewhere in SPEC.md; a later UX pass "confirmed alignment" against its
own 3-point plan without re-reading SPEC.md's actual text, and the same
miss went uncaught a second time. Schema column comments are behavioral
requirements, not decoration — implement them or record them below as an
open gap; don't let them silently drop.

`monthly_records.income_monthly` is a case in point: SPEC.md originally
described it as an annually-entered, monthly-prorated figure (comment
`-- 年次入力を月按分`), with income deliberately absent from the monthly
form. That turned out wrong on reflection — dividing gross annual income
by 12 doesn't match take-home pay (税・社会保険料 are deducted, and bonus
months make even net income lumpy across the year) — so the design changed
to: enter net monthly income directly every month, with quick-fill
buttons for the last `recentIncomeMonths` (3) months so a bonus month has
a comparable reference to copy instead of just "last month". The field is
labeled 収入, not 手取り月収 — it's meant to cover more than salary alone
(e.g. 雑所得), so a salary-specific label undersold what it should capture.
SPEC.md's schema comment doesn't reflect any of this anymore; section 6 does.

## Architecture

Single Go binary, server-rendered with `html/template` — no separate
frontend build, no npm. This is a deliberate choice (SPEC.md §7) to minimize
supply-chain attack surface; don't introduce a JS build pipeline or React.

### DB (sqlc + goose)
`internal/database/migrations/*.sql` (goose-formatted: `-- +goose Up` /
`-- +goose Down`) and `internal/database/query.sql` are the single source of
truth for the DB layer. sqlc reads the migrations directory directly and
ignores Down sections. Workflow:
1. Add a new numbered file under `migrations/` (or edit `query.sql`)
2. `mise run generate` (sqlc) and `mise run db-migrate` (goose, applies to
   `$DATABASE_URL`)

**Never edit `internal/database/sqlcgen/` by hand.** goose itself is a
dev-time tool only (`hack/tools.go`, `go run`) — it is not imported into
`cmd/server`, so applying migrations is an explicit step, not automatic on
app boot.

## Commands
All commands run via `mise run <task>`. Do not invoke `go` directly for
generate/format/test/build — use the mise tasks so generation order and
formatting stay consistent.

All `mise run` tasks may be executed without human confirmation — none of
them push, deploy, or add/upgrade dependencies.

## Development cycle
```
Edit schema.sql / query.sql
  → mise run generate → implement → mise run format → mise run ci
```

## Auth
- `--disable-oidc` skips auth entirely; it's wired into `mise run dev` via
  `.air.toml`. Never use it outside local development.
- Production auth is OIDC (Authorization Code + PKCE, browser session via an
  `HttpOnly`/`Secure` cookie holding the raw ID token — re-verified against
  the issuer's JWKS on every request via `go-oidc`, so no session store or
  extra crypto dependency is needed) plus a fixed allowlist
  (`OIDC_ALLOWED_EMAILS`), since this is a single-owner personal app
  (SPEC.md §2). Implemented in `internal/auth/` (`auth.go`'s `Middleware` +
  `routes.go`'s `/login`, `/callback`, `/logout`), modeled on the sibling
  homelab app `korpus`'s use of `github.com/coreos/go-oidc/v3` +
  `golang.org/x/oauth2`, adapted for a server-rendered app's cookie-session
  flow instead of `korpus`'s SPA-with-bearer-token pattern.
- The shared homelab IdP (Ory Hydra) does not put the user's email in the
  `email` claim — its login-consent app sets the Hydra subject to the email
  address, so `OIDC_ALLOWED_EMAILS` is checked against the verified ID
  token's `Subject` (`sub`), not an `email` claim. Getting this wrong makes
  login silently and permanently fail (the `email` claim is simply never
  present).
- `/mcp` (a possible future MCP server exposure) and `OIDC_AUDIENCE`-based
  resource-token checks are explicitly out of scope for the current
  implementation — `OIDCAudience` is parsed and required but currently
  unused, reserved for that later work.

## NISA NAV data source
SPEC.md 4.1 describes scraping a CSV, but that page turned out to be
JS-rendered with no stable CSV URL. `internal/mufg` instead calls MUFG's
documented, externally-published JSON API (`developer.am.mufg.jp`,
`GET /fund_information_date/fund_cd/{code}/base_date/{YYYYMMDD}`, one day
per call) — confirmed against their published API spec. It requires no API
key. A fund's `funds.isin_or_code` column holds the MUFG `fund_cd` (e.g.
`253266` for eMAXIS Slim 米国株式(S&P500)), not an ISIN.

The `/funds/{id}/sync-nav` web action only syncs a bounded recent window
(`maxSyncWindowDays`) per click, since the API is one-day-per-request and a
full history would be thousands of sequential calls — too slow for a single
HTTP request/response cycle. Run `cmd/backfill-nav` once after registering a
new fund to backfill its full history from inception.

## Running locally
```
mise run dev      # starts Postgres, applies pending migrations, then Air
                   # hot-reload + --disable-oidc on http://localhost:8080
```
`dev` runs `db-up` as its first step, so a fresh checkout needs nothing
else. `mise run db-down` stops the DB; `mise run db-reset` wipes and
recreates it (fresh volume) then re-migrates.

`db-up`/`db-down`/`db-reset` run `tools/devdb` (`go run ./tools/devdb ...`),
a small first-party CLI — not shell script + `docker compose`. Reasons:
- `docker-compose.yml` (the `db` service) stays the single declarative
  source of the dev DB's shape, but `docker compose` doesn't work with
  Apple's native `container` CLI, and the only third-party compose-for-
  `container` plugin (`container-compose`) is flagged by Homebrew as a
  likely supply-chain compromise — not something to depend on given
  SPEC.md §7's stance.
- `tools/devdb` parses `docker-compose.yml`'s `db` service with
  `gopkg.in/yaml.v3` (already an indirect dependency via sqlc, so this adds
  no new trust surface) and translates it into `docker`/`container run`
  invocations — same code path for both engines, since their CLIs share
  enough flag syntax (`-e`, `-p`, `-v`, `--name`) that no per-engine
  branching is needed beyond the binary name. It prefers a reachable Docker
  daemon (Docker Desktop, OrbStack, Colima, …) and falls back to `container`.
- `PGDATA` is set to a subdirectory of the mount (`docker-compose.yml`),
  not the volume's mount root — Apple's `container` formats named volumes
  with a filesystem that pre-populates `lost+found`, which makes Postgres's
  `initdb` refuse to use the mount root directly. Harmless on Docker.

`DATABASE_URL` defaults to `docker-compose.yml`'s dev credentials via
`mise.toml`'s `[env]`; override it if you're pointing at a different DB.

## Testing
`mise run ci` (`go test ./...`) is pure unit tests — no DB required, nothing
touches Postgres.

`mise run test-integration` drives the real HTTP handlers against a real
Postgres end to end (`internal/handler/integration_test.go`, build tag
`integration`). It always targets `docker-compose.yml`'s **db-test**
service — a second container/volume/port (15432), entirely separate from
`db` (5432) — and resets it before every run. This is deliberate: earlier,
manual verification against the shared dev DB (`db`) left stray rows behind
and confused manual checking of the running app. Never point
`test-integration` (or any of your own manual verification) at `db`;
always use `db-test` or an ad hoc scratch container instead, and delete
scratch containers when done — the dev DB a person is looking at in the
browser must stay exactly what they put there.

Excluded from integration tests: anything hitting the real MUFG API
(`POST /funds/{id}/sync-nav`) — network-dependent and slow to run on every
invocation. Tests needing NAV history insert it directly via `sqlcgen`.

## Dependencies
Per SPEC.md §7, keep backend dependencies minimal (DB driver, plus
sqlc/air as dev-only tools) to limit supply-chain attack surface. Don't add
a dependency without first checking whether the standard library already
covers it.
