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

I used straightforward field resolvers for `User.tasks`. With the small seeded dataset, this can be easier to understand than adding a DataLoader. For a larger dataset, I would batch those relationship queries to avoid an N+1 pattern.

### Error and input handling

GraphQL IDs arrive as strings and are validated before being converted to PostgreSQL `BIGINT` values. Task dates are validated and returned as `YYYY-MM-DD`, matching the database `DATE` column and the required GraphQL `String` field. GraphQL task statuses are mapped explicitly to the lowercase PostgreSQL enum values.

Missing records return `null` from nullable queries and useful errors from mutations. Unexpected database errors are logged on the server and returned to clients as a generic error so SQL details are not exposed.


## -- Part 3

I chose `description` as the required new field and also exposed `dueDate`. Both fields were already present in the GraphQL `Task` schema and populated from Postgres. I updated `web/app.js` to request them and render the description and formatted due date on each task card. This made the supplied UI more useful without adding a frontend framework.

