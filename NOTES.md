### Dependency setup

When I first ran the seed script, Go reported missing checksum entries for some of the `pgx` dependencies. I ran `go mod tidy` to resolve the required modules and update `go.mod` and `go.sum`. After that, the project compiled and the seed script ran with an expected error.

## -- Part 1

Error encountered while trying seed script:

" seed: insert task "Draft analytical engine notes": ERROR: syntax error at or near "`" (SQLSTATE 42601)
exit status 1 "

The error pointed to a backtick in the task insert statement. I checked the `INSERT INTO tasks` query and noticed that the column names were wrapped in backticks, which PostgreSQL does not accept. I removed the backticks and ran the seed script twice. After each run, I queried the table counts and confirmed that they remained at 3 users, 7 tasks, and 9 tags rather than being duplicated

## -- Part 2

### Library and structure choices

I chose gqlgen because its schema-first workflow keeps the required GraphQL contract in one place and generates type-safe Go interfaces for the resolvers. I used v0.17.94 because the newer release requires Go 1.26, while this assignment specifies Go 1.25. The generated GraphQL execution code and models are committed, but the schema, resolver implementations, and database code remain handwritten.

I kept the structure small. `cmd/server` configures the HTTP server, GraphQL handler, logging, and CORS. `graph` contains the schema, resolvers, and generated gqlgen files. `internal/store` contains the PostgreSQL queries. The resolvers depend on a small store interface, which keeps database logic out of the GraphQL layer and makes the resolver behavior easy to test.

The PostgreSQL store uses pgxpool. Task creation and tag insertion run in one transaction so a failed tag insert cannot leave a partial task. Task queries preload the owning user because `Task.user` is required, and tags are fetched with one bulk query rather than one query per task.

`User.tasks` uses gqlgen's batch resolver support. gqlgen passes sibling users to one resolver call, which performs one bulk `WHERE user_id = ANY(...)` task query and groups the ordered results by user ID. This is request-execution batching for sibling fields, not a whole-request DataLoader cache.

### Error and input handling

GraphQL IDs arrive as strings and are validated before being converted to PostgreSQL `BIGINT` values. Task dates are validated and returned as `YYYY-MM-DD`, matching the database `DATE` column and the required GraphQL `String` field. GraphQL task statuses are mapped explicitly to the lowercase PostgreSQL enum values.

Missing records return `null` from nullable queries and useful errors from mutations. Unexpected database errors are logged on the server and returned to clients as a generic error so SQL details are not exposed.


## -- Part 3

I chose `description` as the required new field and also exposed `dueDate`. Both fields were already present in the GraphQL `Task` schema and populated from Postgres. I updated `web/app.js` to request them and render the description and formatted due date on each task card. This made the supplied UI more useful without adding a frontend framework.

## API hardening

The root `users` and `tasks` queries support offset pagination with a default limit of 50, a maximum limit of 100, and a nonnegative offset. Results are ordered by ID before `LIMIT` and `OFFSET` are applied. The UI omits these arguments, so it continues to load the first 50 users. Offset pagination can skip or repeat records when concurrent writes occur; that tradeoff is acceptable for this assignment's small dataset.

PostgreSQL integration tests use `TEST_DATABASE_URL`. Each run creates a uniquely named temporary schema, sets that schema as the pooled connections' `search_path`, applies `db/schema.sql`, and drops only that schema during cleanup. If the variable is absent, the integration suite skips cleanly. With the supplied Docker database running, use:

```powershell
$env:TEST_DATABASE_URL='postgres://admin:todo@localhost:5432/homework?sslmode=disable'
go test ./...
```

