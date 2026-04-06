# fitness-backend

RESTful API backend for the FitnessApp. Built with Go and Fiber, backed by PostgreSQL, with Google Gemini-powered AI features for workout plan generation and coaching.

## Features

- **Google OAuth** authentication with JWT session issuance
- **Plan management** — CRUD and cloud sync for workout plans
- **History tracking** — completed workout storage and sync
- **AI plan generation** — Gemini 2.5 Flash generates personalized workout plans
- **AI coach** — multi-turn coaching conversations with function calling (queries user history and plans)
- **Daily token budgets** — per-user Gemini token limits to control API costs
- **Incremental migrations** — schema versioned via sequential SQL files

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.26 |
| Framework | Fiber v2 |
| Database | PostgreSQL 15 |
| Auth | Google OAuth2 + JWT (HS256) |
| AI | Google Gemini 2.5 Flash |
| DB Driver | pgx v5 |
| Testing | testify |

## Prerequisites

- Go 1.26+
- Docker and Docker Compose
- Google Cloud project with OAuth2 credentials
- Gemini API key

## Setup

```bash
cd fitness-backend
go mod download
```

Create a `.env` file in this directory:

```env
PORT=8080

POSTGRES_USER=postgres
POSTGRES_PASSWORD=<password>
POSTGRES_DB=fitnessapp
DATABASE_URL=postgres://postgres:<password>@localhost:5433/fitnessapp?sslmode=disable

GOOGLE_CLIENT_ID=<Google OAuth Client ID>
JWT_SECRET=<long random secret>

GEMINI_API_KEY=<Google Gemini API key>
```

## Running Locally

```bash
# Start PostgreSQL
make db-up

# Run the server
make run
```

The server starts on `http://localhost:8080`.

## Testing

Tests run against a separate test database on port 5434.

```bash
make test
```

## Docker

```bash
# Build image
docker build -t fitness-backend:latest .

# Run container
docker run \
  -e GOOGLE_CLIENT_ID=... \
  -e GEMINI_API_KEY=... \
  -e DATABASE_URL=postgres://... \
  -e JWT_SECRET=... \
  -p 8080:8080 \
  fitness-backend:latest
```

## API Reference

Base path: `/api/v1`

### Public

| Method | Path | Description |
|---|---|---|
| POST | `/auth/google` | Exchange Google ID token for a JWT |
| GET | `/health` | Health check |

### Protected (Bearer token required)

| Method | Path | Description |
|---|---|---|
| GET | `/plans` | Fetch user's saved plans |
| POST | `/plans/sync` | Sync local plans to cloud |
| POST | `/plans/generate` | AI-generate a workout plan |
| GET | `/history` | Fetch completed workouts |
| POST | `/history/sync` | Sync local history to cloud |
| GET | `/settings` | Fetch user settings |
| POST | `/settings/sync` | Sync settings to cloud |
| POST | `/chat` | AI coaching conversation |
| GET | `/ai/usage` | Daily token usage for current user |

### Auth Flow

1. Client sends `{ idToken }` from Google Sign-In to `POST /auth/google`
2. Server validates token against Google's public keys
3. User is upserted into the database
4. Server returns a signed 72-hour JWT
5. Client includes `Authorization: Bearer <token>` on all subsequent requests

## Project Structure

```
cmd/api/
  main.go                 # Entry point
internal/
  app/
    app.go               # Fiber setup and route registration
  db/
    db.go               # PostgreSQL connection, migration runner
  handlers/
    auth/               # Google token exchange
    plans/              # Plan CRUD and sync
    history/            # Workout history
    settings/           # User settings
    ai/                 # Plan generation, chat, token limits
  middleware/
    jwt.go              # Protected route middleware
sql/
  migrations/           # Sequential SQL migration files
exercises.json          # Master list of valid exercises (~1000+)
Dockerfile
docker-compose.yml
Jenkinsfile
```

## Database Migrations

Migrations run automatically on startup and are idempotent.

| File | Description |
|---|---|
| `001_init.sql` | Users, plans, history tables |
| `002_prompts.sql` | AI prompt templates table |
| `003_history_duration.sql` | Adds `duration_seconds` to history |
| `004_settings.sql` | User settings table |
| `005_daily_token_usage.sql` | Daily AI token budget tracking |

## AI Features

**Plan Generation** (`POST /plans/generate`):
- Accepts `{ days, level, goal }` and returns a structured workout plan
- Uses Gemini structured output with JSON schema enforcement
- Validates all exercises against `exercises.json` master list
- Retries up to 3 times if invalid exercise names are returned

**AI Coach** (`POST /chat`):
- Multi-turn conversation with Gemini function calling
- Tools available to the model: `get_workout_history`, `get_saved_plans`, `deliver_final_response`
- Keeps last 6 messages of history to manage context size
- Returns `{ response_type: "chat" | "plan", text, plan_data? }`
