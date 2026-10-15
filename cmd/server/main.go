package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"

	"github.com/Samster101/go-interview-homework/graph"
	"github.com/Samster101/go-interview-homework/internal/store"
)

const defaultDatabaseURL = "postgres://admin:todo@localhost:5432/homework?sslmode=disable"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	postgres, err := store.NewPostgres(context.Background(), databaseURL)
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer postgres.Close()

	graphqlHandler := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{Store: postgres, Logger: logger},
	}))

	mux := http.NewServeMux()
	mux.Handle("/graphql", allowCORS(graphqlHandler))
	mux.Handle("/", playground.Handler("Todo GraphQL", "/graphql"))

	logger.Info("GraphQL server listening", "address", ":8080", "endpoint", "/graphql")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		logger.Error("serve GraphQL API", "error", err)
		os.Exit(1)
	}
}

func allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
