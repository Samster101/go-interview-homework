package graph

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/Samster101/go-interview-homework/graph/model"
	"github.com/Samster101/go-interview-homework/internal/store"
)

type fakeStore struct {
	getUser          func(context.Context, int64) (*model.User, error)
	listUsers        func(context.Context, int, int) ([]*model.User, error)
	getTask          func(context.Context, int64) (*model.Task, error)
	listTasks        func(context.Context, *model.TaskStatus, *int64, int, int) ([]*model.Task, error)
	listTasksByUsers func(context.Context, *model.TaskStatus, []int64) (map[int64][]*model.Task, error)
	createTask       func(context.Context, int64, string, *string, *string, []string) (*model.Task, error)
	updateTaskStatus func(context.Context, int64, model.TaskStatus) (*model.Task, error)
}

func (f *fakeStore) GetUser(ctx context.Context, id int64) (*model.User, error) {
	return f.getUser(ctx, id)
}

func (f *fakeStore) ListUsers(ctx context.Context, limit, offset int) ([]*model.User, error) {
	return f.listUsers(ctx, limit, offset)
}

func (f *fakeStore) GetTask(ctx context.Context, id int64) (*model.Task, error) {
	return f.getTask(ctx, id)
}

func (f *fakeStore) ListTasks(ctx context.Context, status *model.TaskStatus, userID *int64, limit, offset int) ([]*model.Task, error) {
	return f.listTasks(ctx, status, userID, limit, offset)
}

func (f *fakeStore) ListTasksByUserIDs(ctx context.Context, status *model.TaskStatus, userIDs []int64) (map[int64][]*model.Task, error) {
	return f.listTasksByUsers(ctx, status, userIDs)
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
		listTasks: func(_ context.Context, gotStatus *model.TaskStatus, gotUserID *int64, limit, offset int) ([]*model.Task, error) {
			if gotStatus == nil || *gotStatus != status {
				t.Fatalf("unexpected status: %v", gotStatus)
			}
			if gotUserID == nil || *gotUserID != 2 {
				t.Fatalf("unexpected user ID: %v", gotUserID)
			}
			if limit != 25 || offset != 5 {
				t.Fatalf("unexpected pagination: limit=%d offset=%d", limit, offset)
			}
			return []*model.Task{}, nil
		},
	}
	userID := "2"
	if _, err := (&queryResolver{&Resolver{Store: fake}}).Tasks(context.Background(), &status, &userID, 25, 5); err != nil {
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

	t.Run("missing task query returns null", func(t *testing.T) {
		fake := &fakeStore{getTask: func(context.Context, int64) (*model.Task, error) {
			return nil, store.ErrNotFound
		}}
		task, err := (&queryResolver{&Resolver{Store: fake}}).Task(context.Background(), "99")
		if err != nil || task != nil {
			t.Fatalf("Task returned task=%#v error=%v, want nil, nil", task, err)
		}
	})

	t.Run("blank title rejected", func(t *testing.T) {
		fake := &fakeStore{createTask: func(context.Context, int64, string, *string, *string, []string) (*model.Task, error) {
			t.Fatal("store must not be called for a blank title")
			return nil, nil
		}}
		blank := "   "
		if _, err := (&mutationResolver{&Resolver{Store: fake}}).CreateTask(context.Background(), "1", blank, nil, nil, nil); err == nil {
			t.Fatal("CreateTask returned no error for a blank title")
		}
	})

	t.Run("invalid due date format rejected", func(t *testing.T) {
		fake := &fakeStore{createTask: func(context.Context, int64, string, *string, *string, []string) (*model.Task, error) {
			t.Fatal("store must not be called for an invalid due date")
			return nil, nil
		}}
		bad := "01-15-2026"
		if _, err := (&mutationResolver{&Resolver{Store: fake}}).CreateTask(context.Background(), "1", "title", nil, &bad, nil); err == nil {
			t.Fatal("CreateTask returned no error for an invalid due date format")
		}
	})

	t.Run("user not found in createTask", func(t *testing.T) {
		fake := &fakeStore{createTask: func(context.Context, int64, string, *string, *string, []string) (*model.Task, error) {
			return nil, store.ErrNotFound
		}}
		_, err := (&mutationResolver{&Resolver{Store: fake}}).CreateTask(context.Background(), "5", "title", nil, nil, nil)
		if err == nil || err.Error() != "user 5 was not found" {
			t.Fatalf("unexpected user-not-found error: %v", err)
		}
	})
}

func TestPaginationValidation(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		offset int
	}{
		{name: "zero limit", limit: 0},
		{name: "limit above maximum", limit: 101},
		{name: "negative offset", limit: 50, offset: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeStore{listUsers: func(context.Context, int, int) ([]*model.User, error) {
				t.Fatal("store must not be called for invalid pagination")
				return nil, nil
			}}
			if _, err := (&queryResolver{&Resolver{Store: fake}}).Users(context.Background(), tt.limit, tt.offset); err == nil {
				t.Fatal("Users returned no error for invalid pagination")
			}
		})
	}
}

func TestUsersPaginationGraphQLDefaultsAndNullability(t *testing.T) {
	called := false
	fake := &fakeStore{listUsers: func(_ context.Context, limit, offset int) ([]*model.User, error) {
		called = true
		if limit != 50 || offset != 0 {
			t.Fatalf("default pagination = (%d, %d), want (50, 0)", limit, offset)
		}
		return []*model.User{}, nil
	}}
	server := handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: &Resolver{Store: fake}}))
	graphQLClient := client.New(server)
	var response struct {
		Users []struct{ ID string }
	}
	if err := graphQLClient.Post(`query { users { id } }`, &response); err != nil {
		t.Fatalf("query with omitted pagination returned an error: %v", err)
	}
	if !called {
		t.Fatal("query with omitted pagination did not call the store")
	}

	called = false
	err := graphQLClient.Post(
		`query Users($limit: Int) { users(limit: $limit) { id } }`,
		&response,
		client.Var("limit", nil),
	)
	if err == nil {
		t.Fatal("query with an explicit null limit returned no error")
	}
	if called {
		t.Fatal("query with an explicit null limit called the store")
	}
}

func TestUserTasksBatchesAndPreservesParentOrder(t *testing.T) {
	firstTask := &model.Task{ID: "4"}
	secondTask := &model.Task{ID: "7"}
	fake := &fakeStore{
		listTasksByUsers: func(_ context.Context, status *model.TaskStatus, userIDs []int64) (map[int64][]*model.Task, error) {
			if status != nil {
				t.Fatalf("unexpected status: %v", *status)
			}
			if !reflect.DeepEqual(userIDs, []int64{2, 1, 3}) {
				t.Fatalf("unexpected user IDs: %#v", userIDs)
			}
			return map[int64][]*model.Task{
				1: {firstTask, secondTask},
				2: {},
			}, nil
		},
	}

	got, err := (&userResolver{&Resolver{Store: fake}}).Tasks(context.Background(), []*model.User{
		{ID: "2"}, {ID: "1"}, {ID: "3"},
	}, nil)
	if err != nil {
		t.Fatalf("Tasks returned an error: %v", err)
	}
	want := [][]*model.Task{{}, {firstTask, secondTask}, {}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tasks returned %#v, want %#v", got, want)
	}
}

func TestUserTasksGraphQLUsesOneBatch(t *testing.T) {
	batchCalls := 0
	fake := &fakeStore{
		listUsers: func(context.Context, int, int) ([]*model.User, error) {
			return []*model.User{{ID: "1"}, {ID: "2"}, {ID: "3"}}, nil
		},
		listTasksByUsers: func(_ context.Context, _ *model.TaskStatus, userIDs []int64) (map[int64][]*model.Task, error) {
			batchCalls++
			if !reflect.DeepEqual(userIDs, []int64{1, 2, 3}) {
				t.Fatalf("batch user IDs = %#v, want [1 2 3]", userIDs)
			}
			return map[int64][]*model.Task{
				1: {{ID: "10"}},
				2: {},
				3: {{ID: "30"}},
			}, nil
		},
	}
	server := handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: &Resolver{Store: fake}}))
	graphQLClient := client.New(server)
	var response struct {
		Users []struct {
			ID    string
			Tasks []struct{ ID string }
		}
	}
	if err := graphQLClient.Post(`query { users { id tasks { id } } }`, &response); err != nil {
		t.Fatalf("batched user task query returned an error: %v", err)
	}
	if batchCalls != 1 {
		t.Fatalf("batch resolver calls = %d, want 1", batchCalls)
	}
	if len(response.Users) != 3 || len(response.Users[0].Tasks) != 1 || len(response.Users[1].Tasks) != 0 || len(response.Users[2].Tasks) != 1 {
		t.Fatalf("unexpected batched response: %#v", response.Users)
	}
}
