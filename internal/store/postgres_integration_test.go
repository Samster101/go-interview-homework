package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/Samster101/go-interview-homework/graph/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresIntegration(t *testing.T) {
	postgres := newIntegrationPostgres(t)
	ctx := context.Background()

	var userIDs []int64
	for _, user := range []struct{ email, name string }{
		{"alice@example.com", "Alice"},
		{"bob@example.com", "Bob"},
		{"cara@example.com", "Cara"},
	} {
		var id int64
		if err := postgres.pool.QueryRow(ctx,
			`INSERT INTO users (email, name) VALUES ($1, $2) RETURNING id`, user.email, user.name,
		).Scan(&id); err != nil {
			t.Fatalf("insert fixture user: %v", err)
		}
		userIDs = append(userIDs, id)
	}

	type taskFixture struct {
		userID int64
		title  string
		status string
	}
	var taskIDs []int64
	for _, task := range []taskFixture{
		{userIDs[0], "Alice pending", "pending"},
		{userIDs[0], "Alice done", "done"},
		{userIDs[1], "Bob in progress", "in_progress"},
		{userIDs[1], "Bob pending", "pending"},
	} {
		var id int64
		if err := postgres.pool.QueryRow(ctx,
			`INSERT INTO tasks (user_id, title, status) VALUES ($1, $2, $3::task_status) RETURNING id`,
			task.userID, task.title, task.status,
		).Scan(&id); err != nil {
			t.Fatalf("insert fixture task: %v", err)
		}
		taskIDs = append(taskIDs, id)
	}
	if _, err := postgres.pool.Exec(ctx,
		`INSERT INTO task_tags (task_id, tag) VALUES ($1, 'backend'), ($1, 'urgent')`, taskIDs[1],
	); err != nil {
		t.Fatalf("insert fixture tags: %v", err)
	}

	t.Run("create task with tags and nullable values", func(t *testing.T) {
		task, err := postgres.CreateTask(ctx, userIDs[0], "Created task", nil, nil, []string{"api", "go"})
		if err != nil {
			t.Fatalf("CreateTask returned an error: %v", err)
		}
		if task.Description != nil || task.DueDate != nil {
			t.Fatalf("nullable fields were not preserved: description=%v dueDate=%v", task.Description, task.DueDate)
		}
		if !reflect.DeepEqual(task.Tags, []string{"api", "go"}) {
			t.Fatalf("CreateTask tags = %#v, want [api go]", task.Tags)
		}
		if task.User == nil || task.User.ID != idString(userIDs[0]) {
			t.Fatalf("CreateTask user = %#v", task.User)
		}
	})

	t.Run("rolls back after tag insertion failure", func(t *testing.T) {
		var before int
		if err := postgres.pool.QueryRow(ctx, `SELECT count(*) FROM tasks`).Scan(&before); err != nil {
			t.Fatalf("count tasks before failure: %v", err)
		}
		if _, err := postgres.CreateTask(ctx, userIDs[0], "Must roll back", nil, nil, []string{"duplicate", "duplicate"}); err == nil {
			t.Fatal("CreateTask returned no error for duplicate tags")
		}
		var after int
		if err := postgres.pool.QueryRow(ctx, `SELECT count(*) FROM tasks`).Scan(&after); err != nil {
			t.Fatalf("count tasks after failure: %v", err)
		}
		if after != before {
			t.Fatalf("task count after rollback = %d, want %d", after, before)
		}
	})

	t.Run("filters tasks by status and user", func(t *testing.T) {
		status := model.TaskStatusPending
		tasks, err := postgres.ListTasks(ctx, &status, &userIDs[1], 100, 0)
		if err != nil {
			t.Fatalf("ListTasks returned an error: %v", err)
		}
		if len(tasks) != 1 || tasks[0].ID != idString(taskIDs[3]) {
			t.Fatalf("ListTasks returned %#v", taskIDStrings(tasks))
		}
	})

	t.Run("paginates root lists in ID order", func(t *testing.T) {
		users, err := postgres.ListUsers(ctx, 1, 1)
		if err != nil {
			t.Fatalf("ListUsers returned an error: %v", err)
		}
		if len(users) != 1 || users[0].ID != idString(userIDs[1]) {
			t.Fatalf("ListUsers returned %#v", users)
		}

		all, err := postgres.ListTasks(ctx, nil, nil, 100, 0)
		if err != nil {
			t.Fatalf("ListTasks all returned an error: %v", err)
		}
		page, err := postgres.ListTasks(ctx, nil, nil, 2, 1)
		if err != nil {
			t.Fatalf("ListTasks page returned an error: %v", err)
		}
		if !reflect.DeepEqual(taskIDStrings(page), taskIDStrings(all[1:3])) {
			t.Fatalf("ListTasks page = %#v, want %#v", taskIDStrings(page), taskIDStrings(all[1:3]))
		}
	})

	t.Run("bulk loads and groups tasks", func(t *testing.T) {
		grouped, err := postgres.ListTasksByUserIDs(ctx, nil, []int64{userIDs[2], userIDs[0], userIDs[1]})
		if err != nil {
			t.Fatalf("ListTasksByUserIDs returned an error: %v", err)
		}
		if len(grouped[userIDs[2]]) != 0 {
			t.Fatalf("user without tasks returned %#v", grouped[userIDs[2]])
		}
		for _, userID := range userIDs[:2] {
			ids := taskIDStrings(grouped[userID])
			if !strictlyIncreasing(ids) {
				t.Fatalf("tasks for user %d are not ordered: %#v", userID, ids)
			}
		}
	})

	t.Run("updates status and handles missing records", func(t *testing.T) {
		updated, err := postgres.UpdateTaskStatus(ctx, taskIDs[0], model.TaskStatusDone)
		if err != nil {
			t.Fatalf("UpdateTaskStatus returned an error: %v", err)
		}
		if updated.Status != model.TaskStatusDone {
			t.Fatalf("updated status = %s, want DONE", updated.Status)
		}
		if updated.Tags == nil || len(updated.Tags) != 0 {
			t.Fatalf("task without tags returned %#v", updated.Tags)
		}
		if _, err := postgres.GetTask(ctx, 999999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetTask missing error = %v, want ErrNotFound", err)
		}
		if _, err := postgres.UpdateTaskStatus(ctx, 999999, model.TaskStatusDone); !errors.Is(err, ErrNotFound) {
			t.Fatalf("UpdateTaskStatus missing error = %v, want ErrNotFound", err)
		}
	})
}

func newIntegrationPostgres(t *testing.T) *Postgres {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create administration pool: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping test database: %v", err)
	}

	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		admin.Close()
		t.Fatalf("generate schema name: %v", err)
	}
	schemaName := "test_" + hex.EncodeToString(random)
	quotedSchema := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
		t.Fatalf("parse test database URL: %v", err)
	}
	// Set search_path before opening the pool so every connection is isolated.
	config.ConnConfig.RuntimeParams["search_path"] = schemaName
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
		t.Fatalf("create isolated test pool: %v", err)
	}
	// Cleanup drops only the schema created by this test run.
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		admin.Close()
	})

	schemaSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "schema.sql"))
	if err != nil {
		t.Fatalf("read database schema: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schemaSQL)); err != nil {
		t.Fatalf("apply database schema: %v", err)
	}
	return &Postgres{pool: pool}
}

func idString(id int64) string {
	return strconv.FormatInt(id, 10)
}

func taskIDStrings(tasks []*model.Task) []string {
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	return ids
}

func strictlyIncreasing(ids []string) bool {
	for i := 1; i < len(ids); i++ {
		previous, previousErr := strconv.ParseInt(ids[i-1], 10, 64)
		current, currentErr := strconv.ParseInt(ids[i], 10, 64)
		if previousErr != nil || currentErr != nil || previous >= current {
			return false
		}
	}
	return true
}
