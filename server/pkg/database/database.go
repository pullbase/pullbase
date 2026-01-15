package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pullbase/pullbase/server/pkg/logging"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	Dialect      Dialect
	Host         string
	Port         int
	User         string
	Password     string
	DatabaseName string
	SSLMode      string
	Path         string
}

func New(cfg Config) (*sqlx.DB, Dialect, error) {
	if cfg.Dialect == "" {
		cfg.Dialect = DialectSQLite
	}

	var db *sqlx.DB
	var err error

	switch cfg.Dialect {
	case DialectSQLite:
		db, err = newSQLite(cfg)
	case DialectPostgres:
		db, err = newPostgres(cfg)
	default:
		db, err = newSQLite(cfg)
	}

	return db, cfg.Dialect, err
}

func newPostgres(cfg Config) (*sqlx.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DatabaseName, cfg.SSLMode,
	)

	db, err := sqlx.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("error opening PostgreSQL database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("error connecting to PostgreSQL: %w", err)
	}

	logging.Info("connected to PostgreSQL database", "host", cfg.Host, "database", cfg.DatabaseName)
	return db, nil
}

func newSQLite(cfg Config) (*sqlx.DB, error) {
	dbPath := cfg.Path
	if dbPath == "" {
		dbPath = "pullbase.db"
	}

	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("error creating database directory: %w", err)
		}
	}

	connStr := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", dbPath)

	db, err := sqlx.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("error opening SQLite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("error connecting to SQLite: %w", err)
	}

	logging.Info("connected to SQLite database", "path", dbPath)
	return db, nil
}

func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func InitSchema(ctx context.Context, db *sqlx.DB, dialect Dialect, migrationPath string) error {
	path := migrationPathForDialect(migrationPath, dialect)
	switch dialect {
	case DialectPostgres:
		return initPostgresSchema(ctx, db, path)
	case DialectSQLite:
		return initSQLiteSchema(ctx, db, path)
	default:
		return initSQLiteSchema(ctx, db, path)
	}
}

func migrationPathForDialect(migrationPath string, dialect Dialect) string {
	if migrationPath == "" {
		return ""
	}
	const filePrefix = "file://"
	clean := migrationPath
	if strings.HasPrefix(migrationPath, filePrefix) {
		clean = migrationPath[len(filePrefix):]
	}
	candidate := filepath.Join(clean, string(dialect))
	if _, err := os.Stat(candidate); err == nil {
		return filePrefix + candidate
	}
	return migrationPath
}

func initPostgresSchema(ctx context.Context, db *sqlx.DB, migrationPath string) error {
	driver, err := postgres.WithInstance(db.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("error creating PostgreSQL migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(migrationPath, "postgres", driver)
	if err != nil {
		return fmt.Errorf("error creating migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("error running PostgreSQL migrations: %w", err)
	}

	logging.Info("PostgreSQL schema initialized successfully")
	return nil
}

func initSQLiteSchema(ctx context.Context, db *sqlx.DB, migrationPath string) error {
	driver, err := sqlite3.WithInstance(db.DB, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("error creating SQLite migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(migrationPath, "sqlite3", driver)
	if err != nil {
		return fmt.Errorf("error creating migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("error running SQLite migrations: %w", err)
	}

	logging.Info("SQLite schema initialized successfully")
	return nil
}
