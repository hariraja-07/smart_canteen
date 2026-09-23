# smart_canteen

Smart canteen: Go backend + Flutter frontend.

## Backend (Go, port 8080)

Requires a PostgreSQL database reachable via `DATABASE_URL`:

```sh
DATABASE_URL=postgres://smartcanteen:smartcanteen_dev@localhost:5432/smart_canteen
```

Tables (`users`, `menu_items`, `orders`, `order_items`) are created on startup and
the menu is seeded once when empty.

### Run locally

```sh
cd backend
go run .
```

### Run with Docker

```sh
# 1. shared network for the two containers
docker network create smart-canteen-net

# 2. start Postgres (persistent volume keeps data across container restarts)
docker run -d --name smart-canteen-db --network smart-canteen-net \
  -e POSTGRES_USER=smartcanteen \
  -e POSTGRES_PASSWORD=smartcanteen_dev \
  -e POSTGRES_DB=smart_canteen \
  -p 5432:5432 \
  -v smart_canteen_data:/var/lib/postgresql/data \
  postgres:16-alpine

# 3. build and run the backend image (URL uses the DB container name, not localhost)
docker build -t smart-canteen-backend backend/
docker run -d --name backend -p 8080:8080 --network smart-canteen-net \
  -e DATABASE_URL=postgres://smartcanteen:smartcanteen_dev@smart-canteen-db:5432/smart_canteen \
  smart-canteen-backend
```

### Endpoints

| Method | Path        | Description                  |
|--------|-------------|------------------------------|
| GET    | `/`         | hello                        |
| GET    | `/health`   | DB reachability check        |
| GET    | `/api/menu` | list of menu items (JSON)    |

## Frontend (Flutter)

```sh
cd frontend
flutter run
```

Connects to `http://localhost:8080` (`frontend/lib/api.dart`). On a phone over
USB, forward the port first:

```sh
adb reverse tcp:8080 tcp:8080
```