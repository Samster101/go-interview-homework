package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Samster101/go-interview-homework/graph/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) GetUser(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	var databaseID int64
	err := p.pool.QueryRow(ctx,
		`SELECT id, email, name FROM users WHERE id = $1`, id,
	).Scan(&databaseID, &user.Email, &user.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	user.ID = strconv.FormatInt(databaseID, 10)
	return &user, nil
}

func (p *Postgres) ListUsers(ctx context.Context) ([]*model.User, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, email, name FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]*model.User, 0)
	for rows.Next() {
		var user model.User
		var id int64
		if err := rows.Scan(&id, &user.Email, &user.Name); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		user.ID = strconv.FormatInt(id, 10)
		users = append(users, &user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (p *Postgres) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	tasks, err := p.queryTasks(ctx, `WHERE t.id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, ErrNotFound
	}
	return tasks[0], nil
}

func (p *Postgres) ListTasks(ctx context.Context, status *model.TaskStatus, userID *int64) ([]*model.Task, error) {
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if status != nil {
		databaseStatus, err := statusToDatabase(*status)
		if err != nil {
			return nil, err
		}
		args = append(args, databaseStatus)
		conditions = append(conditions, fmt.Sprintf("t.status = $%d::task_status", len(args)))
	}
	if userID != nil {
		args = append(args, *userID)
		conditions = append(conditions, fmt.Sprintf("t.user_id = $%d", len(args)))
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	return p.queryTasks(ctx, where, args...)
}

func (p *Postgres) CreateTask(ctx context.Context, userID int64, title string, description, dueDate *string, tags []string) (*model.Task, error) {
	// Create the task and its tags atomically to prevent partial writes.
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create task: %w", err)
	}
	defer tx.Rollback(ctx)

	var userExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&userExists); err != nil {
		return nil, fmt.Errorf("check task user: %w", err)
	}
	if !userExists {
		return nil, ErrNotFound
	}

	var taskID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO tasks (user_id, title, description, due_date)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, title, description, dueDate,
	).Scan(&taskID); err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	for _, tag := range tags {
		if _, err := tx.Exec(ctx, `INSERT INTO task_tags (task_id, tag) VALUES ($1, $2)`, taskID, tag); err != nil {
			return nil, fmt.Errorf("insert task tag: %w", err)
		}
	}

	task, err := queryOneTask(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create task: %w", err)
	}
	return task, nil
}

func (p *Postgres) UpdateTaskStatus(ctx context.Context, id int64, status model.TaskStatus) (*model.Task, error) {
	databaseStatus, err := statusToDatabase(status)
	if err != nil {
		return nil, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin update task: %w", err)
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx,
		`UPDATE tasks SET status = $1::task_status, updated_at = now() WHERE id = $2`,
		databaseStatus, id,
	)
	if err != nil {
		return nil, fmt.Errorf("update task status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil, ErrNotFound
	}

	task, err := queryOneTask(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update task: %w", err)
	}
	return task, nil
}

type taskQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func queryOneTask(ctx context.Context, querier taskQuerier, id int64) (*model.Task, error) {
	tasks, err := queryTasks(ctx, querier, `WHERE t.id = $1`, id)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, ErrNotFound
	}
	return tasks[0], nil
}

func (p *Postgres) queryTasks(ctx context.Context, where string, args ...any) ([]*model.Task, error) {
	return queryTasks(ctx, p.pool, where, args...)
}

func queryTasks(ctx context.Context, querier taskQuerier, where string, args ...any) ([]*model.Task, error) {
	// Preload task owners to avoid additional queries in Task.user.
	query := `SELECT t.id, t.title, t.description, t.status::text, t.due_date::text,
	                 u.id, u.email, u.name
	          FROM tasks t
	          JOIN users u ON u.id = t.user_id ` + where + ` ORDER BY t.id`
	rows, err := querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]*model.Task, 0)
	taskIDs := make([]int64, 0)
	for rows.Next() {
		var task model.Task
		var taskID, userID int64
		var databaseStatus string
		var description, dueDate *string
		var user model.User
		if err := rows.Scan(
			&taskID, &task.Title, &description, &databaseStatus, &dueDate,
			&userID, &user.Email, &user.Name,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		status, err := statusFromDatabase(databaseStatus)
		if err != nil {
			return nil, err
		}
		task.ID = strconv.FormatInt(taskID, 10)
		task.Description = description
		task.DueDate = dueDate
		task.Status = status
		task.Tags = []string{}
		user.ID = strconv.FormatInt(userID, 10)
		task.User = &user
		tasks = append(tasks, &task)
		taskIDs = append(taskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	if err := attachTags(ctx, querier, tasks, taskIDs); err != nil {
		return nil, err
	}
	return tasks, nil
}

func attachTags(ctx context.Context, querier taskQuerier, tasks []*model.Task, taskIDs []int64) error {
	if len(taskIDs) == 0 {
		return nil
	}
	// Fetch tags in bulk to avoid one query per task.
	rows, err := querier.Query(ctx,
		`SELECT task_id, tag FROM task_tags WHERE task_id = ANY($1::bigint[]) ORDER BY task_id, tag`,
		taskIDs,
	)
	if err != nil {
		return fmt.Errorf("query task tags: %w", err)
	}
	defer rows.Close()

	byID := make(map[int64]*model.Task, len(tasks))
	for i, id := range taskIDs {
		byID[id] = tasks[i]
	}
	for rows.Next() {
		var taskID int64
		var tag string
		if err := rows.Scan(&taskID, &tag); err != nil {
			return fmt.Errorf("scan task tag: %w", err)
		}
		if task := byID[taskID]; task != nil {
			task.Tags = append(task.Tags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate task tags: %w", err)
	}
	return nil
}

func statusToDatabase(status model.TaskStatus) (string, error) {
	// Map GraphQL status values to the database's lowercase enum values.
	switch status {
	case model.TaskStatusPending:
		return "pending", nil
	case model.TaskStatusInProgress:
		return "in_progress", nil
	case model.TaskStatusDone:
		return "done", nil
	default:
		return "", fmt.Errorf("invalid task status %q", status)
	}
}

func statusFromDatabase(status string) (model.TaskStatus, error) {
	switch status {
	case "pending":
		return model.TaskStatusPending, nil
	case "in_progress":
		return model.TaskStatusInProgress, nil
	case "done":
		return model.TaskStatusDone, nil
	default:
		return "", fmt.Errorf("unknown database task status %q", status)
	}
}
