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

	"identity-service/services/identity/internal/domain"
)

func TestUserRepoPersistsSignupAggregateAndOutbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	migrations, err := filepath.Glob("../../../db/migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("identity migrations: files=%d err=%v", len(migrations), err)
	}
	container, err := mysqlcontainer.Run(ctx, "mysql:8.0",
		mysqlcontainer.WithDatabase("identity_repo"),
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

	repo := NewUserRepo(db)
	created, err := repo.CreateUser(ctx, "+821055500001", domain.Profile{Nickname: "integration"}, domain.StatusActive)
	if err != nil {
		t.Fatal(err)
	}
	found, err := repo.FindByCredential(ctx, domain.AuthTypePhone, "+821055500001")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != created.ID || found.Status != domain.StatusActive {
		t.Fatalf("found user = %+v, created = %+v", found, created)
	}
	profile, err := repo.GetProfile(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Nickname != "integration" {
		t.Fatalf("nickname = %q", profile.Nickname)
	}
	var eventType string
	var aggregateID string
	if err := db.QueryRowContext(ctx, `SELECT event_type, aggregate_id FROM outbox WHERE aggregate_id = ?`, created.ID).
		Scan(&eventType, &aggregateID); err != nil {
		t.Fatal(err)
	}
	if eventType != "user.created" {
		t.Fatalf("event type = %q", eventType)
	}
}
