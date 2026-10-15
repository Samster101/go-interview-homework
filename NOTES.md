### Dependency setup

When I first ran the seed script, Go reported missing checksum entries for some of the `pgx` dependencies. I ran `go mod tidy` to resolve the required modules and update `go.mod` and `go.sum`. After that, the project compiled and the seed script ran with an expected error.

## -- Part 1
Error encountered while trying seed script:

" seed: insert task "Draft analytical engine notes": ERROR: syntax error at or near "`" (SQLSTATE 42601)
exit status 1 "

The error pointed to a backtick in the task insert statement. I checked the `INSERT INTO tasks` query and noticed that the column names were wrapped in backticks, which PostgreSQL does not accept. I removed the backticks and ran the seed script twice. After each run, I queried the table counts and confirmed that they remained at 3 users, 7 tasks, and 9 tags rather than being duplicated

