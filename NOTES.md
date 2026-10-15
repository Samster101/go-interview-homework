# Implementation Notes

## AI Usage

I used AI extensively throughout this assignment for planning, learning, code generation, and implementation support. I started by breaking down the requirements, familiarizing myself with the technologies involved, and evaluating different approaches before settling on an implementation plan that prioritized the core functionality.

Rather than generating the entire application at once, I worked incrementally, building and testing one component at a time. AI helped generate significant portions of the initial code, which I then reviewed, modified, and tested to understand how each piece worked and how it fit into the overall application. This iterative approach allowed me to catch issues, refine the implementation, and build on working functionality rather than introducing too many changes at once.

Once the core requirements were met, I used AI to help evaluate potential improvements, including resolver timing, pagination, and integration tests. I prioritized additions that provided practical value within the assignment's scope and time constraints.

Throughout the process, I remained involved in the architectural decisions, implementation tradeoffs, debugging, and validation, using AI as a development tool while taking responsibility for understanding and verifying the final code.

## 1. Database and Seed

The seed script failed with PostgreSQL error `42601` due to backticks around column names in an `INSERT INTO tasks` statement. I located the query using the error message, removed the backticks, and verified that running the seed twice produced the same 3 users, 7 tasks, and 10 tags.

I used truncate-and-reseed for deterministic local fixtures. This is simpler than upserts but would not be appropriate for preserving shared or production data.

## 2. GraphQL API

I chose **gqlgen v0.17.94** for its schema-first design and generated type-safe resolvers. I pinned this version for Go 1.25 compatibility; the tradeoff is maintaining generated code as the schema changes.

The project uses three main packages:
- `cmd/server`: HTTP server, GraphQL handler, logging, and CORS.
- `graph`: GraphQL schema, resolvers, and generated code.
- `internal/store`: PostgreSQL operations using `pgxpool` and handwritten SQL.

A small store interface separates SQL from GraphQL logic and allows resolver unit testing, at the cost of a small amount of extra plumbing compared to calling `pgxpool` directly from resolvers.

Task creation uses a transaction to ensure tasks and tags are inserted atomically. Task queries preload owners and bulk-load tags to reduce database queries. `internal/store` uses `pgxpool` with handwritten SQL rather than an ORM, which keeps the ordering, filtering, and rollback behavior explicit at the cost of more manual scan and mapping code.

gqlgen's native batch resolver consolidates sibling `User.tasks` requests into a single bulk query without an additional DataLoader dependency. Its limitation is that it only batches sibling fields within one request; a DataLoader would add request-wide deduplication across multiple resolution paths, which is not needed here.

Each resolver logs its name and duration in milliseconds via a deferred `slog` call, so timing appears in the structured JSON output alongside error logs.

GraphQL IDs are validated as positive PostgreSQL `BIGINT` values, statuses are mapped to database enums, and dates use `YYYY-MM-DD`. Missing records return `null` for nullable queries or an error for mutations. Unexpected database errors are logged without exposing SQL details to clients.

## 3. UI and API Improvements

I exposed both `description` and `dueDate` in the existing JavaScript UI. Both were already stored in PostgreSQL, so this required only GraphQL query and rendering changes.

Root `users` and `tasks` queries support limit/offset pagination with a default of 50, a maximum of 100, and deterministic ID ordering. I chose offset pagination for simplicity, accepting that concurrent writes may cause skipped or repeated records.

## 4. Testing and Tradeoffs

Resolver unit tests cover input validation and GraphQL-to-store behavior including not-found handling and default arguments. PostgreSQL integration tests verify filtering, pagination, relationship batching, nullable field persistence, transactions, and rollback using isolated temporary schemas.

Integration tests use `TEST_DATABASE_URL` and a unique temporary PostgreSQL schema, which is removed afterward. Tests skip when the variable is absent.

**Run integration tests with PostgreSQL running:**

```powershell
$env:TEST_DATABASE_URL='postgres://admin:todo@localhost:5432/homework?sslmode=disable'
go test ./...
```

I intentionally skipped cursor pagination, request-wide caching, and deployment configuration. Further production improvements would include authentication, graceful shutdown, health checks, and distributed tracing.

## 5. Running the Application

From a fresh clone:

```powershell
git clone https://github.com/Samster101/go-interview-homework.git
cd go-interview-homework

docker compose up -d
go run ./cmd/seed
```

In a separate terminal, start the API:

```powershell
go run ./cmd/server
```

The GraphQL playground is available at `http://localhost:8080`, and the API is at `http://localhost:8080/graphql`.

To serve the UI from another terminal:

```powershell
python -m http.server 8081 --directory web
```

Open `http://localhost:8081`.

Reviewed by: AS
