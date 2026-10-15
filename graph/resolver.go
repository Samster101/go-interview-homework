package graph

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Samster101/go-interview-homework/graph/model"
)

// TaskStore defines the data access methods required by the GraphQL resolvers.
type TaskStore interface {
	GetUser(ctx context.Context, id int64) (*model.User, error)
	ListUsers(ctx context.Context, limit, offset int) ([]*model.User, error)
	GetTask(ctx context.Context, id int64) (*model.Task, error)
	ListTasks(ctx context.Context, status *model.TaskStatus, userID *int64, limit, offset int) ([]*model.Task, error)
	ListTasksByUserIDs(ctx context.Context, status *model.TaskStatus, userIDs []int64) (map[int64][]*model.Task, error)
	CreateTask(ctx context.Context, userID int64, title string, description, dueDate *string, tags []string) (*model.Task, error)
	UpdateTaskStatus(ctx context.Context, id int64, status model.TaskStatus) (*model.Task, error)
}

func validatePagination(limit, offset int) error {
	if limit < 1 || limit > 100 {
		return errors.New("limit must be between 1 and 100")
	}
	if offset < 0 {
		return errors.New("offset must be nonnegative")
	}
	return nil
}

// Resolver is the root GraphQL resolver, holding shared dependencies.
type Resolver struct {
	Store  TaskStore
	Logger *slog.Logger
}

func parseID(value string) (int64, error) {
	// GraphQL IDs are strings, while PostgreSQL uses positive BIGINT keys.
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("must be a positive integer")
	}
	return id, nil
}

func normalizeTags(tags []string) ([]string, error) {
	// Normalize tags to prevent duplicate primary-key violations.
	clean := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, value := range tags {
		tag := strings.TrimSpace(value)
		if tag == "" {
			return nil, errors.New("tags must not contain blank values")
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		clean = append(clean, tag)
	}
	return clean, nil
}

func (r *Resolver) internalError(operation string, err error) error {
	// Avoid exposing internal database errors through GraphQL responses.
	if r.Logger != nil {
		r.Logger.Error(operation, "error", err)
	}
	return errors.New("an internal error occurred")
}

func (r *Resolver) logTiming(field string, start time.Time) {
	if r.Logger != nil {
		r.Logger.Info("resolver", "field", field, "duration_ms", time.Since(start).Milliseconds())
	}
}
