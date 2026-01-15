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
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
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

	db, err := sqlx.Open("sqlite", connStr)
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

	if err := checkSQLiteVersion(db); err != nil {
		logging.Warn("SQLite version check failed", "error", err)
	}

	logging.Info("connected to SQLite database", "path", dbPath)
	return db, nil
}

func checkSQLiteVersion(db *sqlx.DB) error {
	var version string
	if err := db.Get(&version, "SELECT sqlite_version()"); err != nil {
		return fmt.Errorf("failed to query SQLite version: %w", err)
	}

	major, minor, patch, err := parseSQLiteVersion(version)
	if err != nil {
		return fmt.Errorf("failed to parse SQLite version %q: %w", version, err)
	}

	const minMajor, minMinor = 3, 35
	if major < minMajor || (major == minMajor && minor < minMinor) {
		return fmt.Errorf("SQLite version %s is older than required 3.35.0 (needed for ALTER TABLE DROP COLUMN in migrations)", version)
	}

	logging.Debug("SQLite version check passed", "version", version, "major", major, "minor", minor, "patch", patch)
	return nil
}

func parseSQLiteVersion(version string) (major, minor, patch int, err error) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0, 0, 0, fmt.Errorf("invalid version format")
	}

	if _, err := fmt.Sscanf(parts[0], "%d", &major); err != nil {
		return 0, 0, 0, err
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minor); err != nil {
		return 0, 0, 0, err
	}
	if len(parts) >= 3 {
		fmt.Sscanf(parts[2], "%d", &patch)
	}
	return major, minor, patch, nil
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
	driver, err := sqlite.WithInstance(db.DB, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("error creating SQLite migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(migrationPath, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("error creating migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("error running SQLite migrations: %w", err)
	}

	logging.Info("SQLite schema initialized successfully")
	return nil
}
