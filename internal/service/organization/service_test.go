package database

import (
	"database/sql"
	"testing"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func NewTestService(t *testing.T) *Service {
	db, err := sql.Open("mysql", "root:notsecret@tcp(127.0.0.1:3309)/admin_test")
	if err != nil {
		t.Errorf("unable to open database connection: %v", err)
		t.FailNow()
	}

	// Allow only one open connection for test to ensure there aren't
	// any concurrency issues or transaction bugs.
	db.SetMaxOpenConns(1)

	// Execute migrations
	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		t.Errorf("error configuring migrations: %v", err)
		t.FailNow()
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../../migrations", "mysql", driver)
	if err != nil {
		t.Errorf("error applying migrations: %v", err)
		t.FailNow()
	}
	m.Steps(2)

	dialect := goqu.Dialect("mysql")
	return NewService(dialect.DB(db))
}
