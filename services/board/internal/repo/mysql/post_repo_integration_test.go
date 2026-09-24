//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"

	"identity-service/services/board/internal/domain"
)

func TestPostRepoCreatesAndListsWithLocalAuthorReadModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	migrations, err := filepath.Glob("../../../db/migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("board migrations: files=%d err=%v", len(migrations), err)
	}
	container, err := mysqlcontainer.Run(ctx, "mysql:8.0",
		mysqlcontainer.WithDatabase("board_repo"),
		mysqlcontainer.WithUsername("test"),
		mysqlcontainer.WithPassword("test"),
		mysqlcontainer.WithScripts(migrations...),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO authors (user_id, nickname) VALUES ('42', 'local-author')`); err != nil {
		t.Fatal(err)
	}

	repo := NewPostRepo(db)
	created, err := repo.Create(ctx, &domain.Post{AuthorID: "42", Title: "integration", Content: "real mysql"})
	if err != nil {
		t.Fatal(err)
	}
	if created.AuthorNickname != "local-author" {
		t.Fatalf("author nickname = %q", created.AuthorNickname)
	}
	posts, err := repo.List(ctx, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].ID != created.ID {
		t.Fatalf("posts = %+v", posts)
	}
}
