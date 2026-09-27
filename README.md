# smart_canteen

Smart canteen: Go backend, PostgreSQL, Flutter frontend.

Students pay for food with coins instead of cash. Coins are bought at the
counter, spent on orders, and every movement is written to an append-only
ledger. Nothing here is a shortcut around that: a balance is a cache of the
ledger, never the source of truth.

## The rules this system keeps

- **1 coin = 1 rupee, always.** Prices and balances are whole numbers, enforced
  by a `CHECK` constraint, not by convention.
- **No fractional money.** The API rejects `9.99` rather than rounding it.
- **Registration is closed.** Accounts are created by an admin through the
  `createuser` CLI. There is no signup endpoint, because an open signup on a
  system that mints spendable currency is an invitation.
- **The ledger is the source of truth.** `users.coin_balance` is a cache. Every
  balance change goes through `applyLedgerEntry`, which writes the ledger and
  the cache in one transaction.
- **A refund is a real pair of entries**, not a deleted row. Coins returned to a
  customer are visible as `refund` entries on both sides of the transfer.
- **A client never sets a price.** An order carries dish ids and quantities, and
  the server looks up the price. The item name and price are snapshotted onto
  the order so a later menu edit cannot rewrite history.

## Roles

| Role | Can do |
|------|--------|
| `admin` | everything: mint coins, view all orders, run the kitchen, read any coin history |
| `canteen_management` | view all orders, advance order status, read the canteen's own history |
| `student` | place orders, see own orders, see own coin history |
| `staff` | same as `student` |

Roles come from the server. The Flutter app hides screens a role cannot use, but
that is a courtesy to the user: the server rejects the request regardless.

## Backend (Go, port 8080)

### Configuration

Copy `backend/.env.example` to `backend/.env` and fill it in. `.env` is ignored
by git.

| Variable | Required | Notes |
|----------|----------|-------|
| `DATABASE_URL` | yes | PostgreSQL connection string |
| `JWT_SECRET` | yes | at least 32 characters; signs every login token |
| `DEMO_USER_PASSWORD` | no | when set, startup seeds five demo accounts sharing this password. **Leave unset in production.** |
| `TEST_DATABASE_URL` | no | used by the Go tests only |

### Run locally

```sh
cd backend
go run .
```

### Commands

`serve` is the default. The rest are for administration:

```sh
# password is read from stdin when --password is omitted, so it never lands in
# your shell history
echo 'some-password' | go run . createuser \
  --email ravi@college.edu --name Ravi --role student --coins 100

# bulk import: shared password on the first line, then email,name,role,coins
cat students.csv | go run . createusers

go run . reconcile                    # audit: balances vs ledger, transfer pairing
```

`reconcile` is the one to run when something looks wrong. It checks that every
cached balance equals the sum of that user's ledger, and that every transfer
group of more than one entry nets to zero. A single-entry exchange is exempt
from that second rule, since minting is a one-sided entry by design. Reported
spend and revenue are net of refunds, so a cancelled order does not read as
revenue.

### Database

Tables are created by a versioned migration runner (`schema_migrations`), not by
ad-hoc `CREATE TABLE` on boot. Each migration runs in its own transaction and is
applied exactly once, so a deploy cannot half-apply and a rollback story stays
honest. Migrations are append-only: to change one, add the next.

## Endpoints

| Method | Path | Who | Purpose |
|--------|------|-----|---------|
| GET | `/health` | anyone | database reachability |
| GET | `/api/menu` | anyone | menu items with whole-coin prices |
| POST | `/api/auth/login` | anyone | exchange email and password for a token |
| GET | `/api/me` | any signed-in | the caller's own profile and balance |
| GET | `/api/orders` | any signed-in | own orders; whole queue for admin and canteen |
| POST | `/api/orders` | any signed-in | place an order, paid in coins |
| PATCH | `/api/orders/{id}/status` | admin, canteen | advance or cancel an order |
| GET | `/api/users/{id}/coins` | owner, admin, canteen | coin ledger history |
| GET | `/api/admin/users` | admin | the account roster |
| POST | `/api/admin/users/{id}/coins` | admin | exchange cash for coins |

Order status moves `pending → preparing → ready → completed`, and any of those
may be cancelled, which writes a refund. Moving an order backwards is a `409`,
not a silent success.

## Tests

The backend tests need a real database, because most of what matters here is
database behaviour: row locks, transaction boundaries, and constraints.

```sh
docker exec -it smart-canteen-db createdb -U smartcanteen sc_test
cd backend
TEST_DATABASE_URL='postgres://smartcanteen:change-me@localhost:5432/sc_test' \
  go test -race -shuffle=on ./...
```

`TEST_DATABASE_URL` is the only variable the suite reads. It deliberately does
not fall back to `DATABASE_URL`: the tests write balances and inject drift on
purpose, so a routine `go test` must never be able to point itself at a real
database. Point it at a throwaway one.

Without it, the 18 database tests skip and say so, while the rest still run. An
earlier version exited before running anything at all, so a CI job that lost its
database secret reported `ok` having executed zero tests.

Tests create their own users with a per-run id, so reruns stay green against the
same database. Every test that moves money asserts the full set of ledger
invariants afterwards.

`TestConcurrentOrdersCannotOverspend` is the regression test for a real deadlock:
it races enough orders to spend the same coins and fails if the customer's row
lock is not taken before the order row is inserted.

Frontend:

```sh
cd frontend
flutter test
```

## Frontend (Flutter)

```sh
cd frontend
flutter run
```

Connects to `http://localhost:8080` (`frontend/lib/api.dart`). On a physical
phone over USB, forward the port first, or the app will find no server:

```sh
adb reverse tcp:8080 tcp:8080
```

Screens are chosen by role: menu, cart and checkout for everyone; the whole order
queue with status controls for the canteen and admin; the roster and coin
exchange for the admin. A request that fails is answered by what went wrong, not
by one generic error: not enough coins says to visit the counter, a server fault
says to retry, and an expired token returns to login.

## Docker

```sh
docker network create smart-canteen-net

docker run -d --name smart-canteen-db --network smart-canteen-net \
  -e POSTGRES_USER=smartcanteen \
  -e POSTGRES_PASSWORD=change-me \
  -e POSTGRES_DB=smart_canteen \
  -p 5432:5432 \
  -v smart_canteen_data:/var/lib/postgresql/data \
  postgres:16-alpine

docker build -t smart-canteen-backend backend/
docker run -d --name backend -p 8080:8080 --network smart-canteen-net \
  -e DATABASE_URL=postgres://smartcanteen:change-me@smart-canteen-db:5432/smart_canteen \
  -e JWT_SECRET=at-least-32-characters-of-real-randomness \
  smart-canteen-backend
```

The database URL uses the container name, not `localhost`, because `localhost`
inside the backend container is the backend container.

## Deploying

The backend is a single stateless binary that talks to PostgreSQL, so it scales
horizontally as long as exactly one instance runs migrations at a time.

Required on any host:

- `DATABASE_URL` pointing at a real PostgreSQL database
- `JWT_SECRET` of at least 32 random characters
- `DEMO_USER_PASSWORD` **unset**

On Render, create a PostgreSQL instance, a web service pointing at
`backend/Dockerfile`, and set those two variables. The service reads the port
from `PORT`, which is what Render injects; an unusable value fails fast instead
of silently falling back, and with `PORT` unset it binds 8080 for local use.

The Flutter app is a normal mobile build. It points at a hardcoded
`Api.baseUrl`, so change that constant to the deployed backend URL before
building. Cleartext HTTP is enabled in the Android manifest for local
development; a production build talking to a public host should use HTTPS and
that setting should be tightened.
