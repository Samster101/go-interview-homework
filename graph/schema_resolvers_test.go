package graph

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Samster101/go-interview-homework/graph/model"
	"github.com/Samster101/go-interview-homework/internal/store"
)

type fakeStore struct {
	getUser          func(context.Context, int64) (*model.User, error)
	listUsers        func(context.Context) ([]*model.User, error)
	getTask          func(context.Context, int64) (*model.Task, error)
	listTasks        func(context.Context, *model.TaskStatus, *int64) ([]*model.Task, error)
	createTask       func(context.Context, int64, string, *string, *string, []string) (*model.Task, error)
	updateTaskStatus func(context.Context, int64, model.TaskStatus) (*model.Task, error)
}

func (f *fakeStore) GetUser(ctx context.Context, id int64) (*model.User, error) {
	return f.getUser(ctx, id)
}

func (f *fakeStore) ListUsers(ctx context.Context) ([]*model.User, error) {
	return f.listUsers(ctx)
}

func (f *fakeStore) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	return f.getTask(ctx, id)
}

func (f *fakeStore) ListTasks(ctx context.Context, status *model.TaskStatus, userID *int64) ([]*model.Task, error) {
	return f.listTasks(ctx, status, userID)
}

func (f *fakeStore) CreateTask(ctx context.Context, userID int64, title string, description, dueDate *string, tags []string) (*model.Task, error) {
	return f.createTask(ctx, userID, title, description, dueDate, tags)
}

func (f *fakeStore) UpdateTaskStatus(ctx context.Context, id int64, status model.TaskStatus) (*model.Task, error) {
	return f.updateTaskStatus(ctx, id, status)
}

func TestCreateTaskNormalizesInput(t *testing.T) {
	description := "Write the implementation notes"
	dueDate := " 2026-08-01 "
	want := &model.Task{ID: "8", Title: "Draft notes", Status: model.TaskStatusPending}
	fake := &fakeStore{
		createTask: func(_ context.Context, userID int64, title string, gotDescription, gotDueDate *string, tags []string) (*model.Task, error) {
			if userID != 3 || title != "Draft notes" {
				t.Fatalf("unexpected task input: userID=%d title=%q", userID, title)
			}
			if gotDescription == nil || *gotDescription != description {
				t.Fatalf("unexpected description: %v", gotDescription)
			}
			if gotDueDate == nil || *gotDueDate != "2026-08-01" {
				t.Fatalf("unexpected due date: %v", gotDueDate)
			}
			if !reflect.DeepEqual(tags, []string{"writing", "go"}) {
				t.Fatalf("unexpected tags: %#v", tags)
			}
			return want, nil
		},
	}

	got, err := (&mutationResolver{&Resolver{Store: fake}}).CreateTask(
		context.Background(), "3", "  Draft notes  ", &description, &dueDate,
		[]string{" writing ", "go", "writing"},
	)
	if err != nil {
		t.Fatalf("CreateTask returned an error: %v", err)
	}
	if got != want {
		t.Fatalf("CreateTask returned %#v, want %#v", got, want)
	}
}

func TestTasksPassesCombinedFilters(t *testing.T) {
	status := model.TaskStatusInProgress
	fake := &fakeStore{
		listTasks: func(_ context.Context, gotStatus *model.TaskStatus, gotUserID *int64) ([]*model.Task, error) {
			if gotStatus == nil || *gotStatus != status {
				t.Fatalf("unexpected status: %v", gotStatus)
			}
			if gotUserID == nil || *gotUserID != 2 {
				t.Fatalf("unexpected user ID: %v", gotUserID)
			}
			return []*model.Task{}, nil
		},
	}
	userID := "2"
	if _, err := (&queryResolver{&Resolver{Store: fake}}).Tasks(context.Background(), &status, &userID); err != nil {
		t.Fatalf("Tasks returned an error: %v", err)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	want := &model.Task{ID: "4", Status: model.TaskStatusDone}
	fake := &fakeStore{
		updateTaskStatus: func(_ context.Context, id int64, status model.TaskStatus) (*model.Task, error) {
			if id != 4 || status != model.TaskStatusDone {
				t.Fatalf("unexpected update: id=%d status=%s", id, status)
			}
			return want, nil
		},
	}

	got, err := (&mutationResolver{&Resolver{Store: fake}}).UpdateTaskStatus(context.Background(), "4", model.TaskStatusDone)
	if err != nil {
		t.Fatalf("UpdateTaskStatus returned an error: %v", err)
	}
	if got != want {
		t.Fatalf("UpdateTaskStatus returned %#v, want %#v", got, want)
	}
}

func TestInvalidInputAndMissingRecords(t *testing.T) {
	t.Run("invalid ID", func(t *testing.T) {
		fake := &fakeStore{getTask: func(context.Context, int64) (*model.Task, error) {
			t.Fatal("store must not be called for an invalid ID")
			return nil, nil
		}}
		if _, err := (&queryResolver{&Resolver{Store: fake}}).Task(context.Background(), "abc"); err == nil {
			t.Fatal("Task returned no error for an invalid ID")
		}
	})

	t.Run("missing query record", func(t *testing.T) {
		fake := &fakeStore{getUser: func(context.Context, int64) (*model.User, error) {
			return nil, store.ErrNotFound
		}}
		user, err := (&queryResolver{&Resolver{Store: fake}}).User(context.Background(), "99")
		if err != nil || user != nil {
			t.Fatalf("User returned user=%#v error=%v, want nil, nil", user, err)
		}
	})

	t.Run("missing mutation record", func(t *testing.T) {
		fake := &fakeStore{updateTaskStatus: func(context.Context, int64, model.TaskStatus) (*model.Task, error) {
			return nil, store.ErrNotFound
		}}
		_, err := (&mutationResolver{&Resolver{Store: fake}}).UpdateTaskStatus(context.Background(), "99", model.TaskStatusDone)
		if err == nil || !errors.Is(err, store.ErrNotFound) && err.Error() != "task 99 was not found" {
			t.Fatalf("unexpected missing-task error: %v", err)
		}
	})
}
