package database

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectSQLite   Dialect = "sqlite"
)

func (d Dialect) String() string {
	return string(d)
}

func (d Dialect) DriverName() string {
	switch d {
	case DialectSQLite:
		return "sqlite3"
	case DialectPostgres:
		return "postgres"
	default:
		return "sqlite3"
	}
}

func (d Dialect) BindType() int {
	switch d {
	case DialectSQLite:
		return sqlx.QUESTION
	case DialectPostgres:
		return sqlx.DOLLAR
	default:
		return sqlx.QUESTION
	}
}

func ParseDialect(s string) (Dialect, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "postgres", "postgresql", "pg":
		return DialectPostgres, nil
	case "sqlite", "sqlite3", "":
		return DialectSQLite, nil
	default:
		return "", fmt.Errorf("unsupported database dialect: %s", s)
	}
}

func (d Dialect) Rebind(query string) string {
	return sqlx.Rebind(d.BindType(), query)
}

func (d Dialect) SupportsReturning() bool {
	return d == DialectPostgres
}

func (d Dialect) TimestampType() string {
	switch d {
	case DialectPostgres:
		return "TIMESTAMP WITH TIME ZONE"
	case DialectSQLite:
		return "DATETIME"
	default:
		return "DATETIME"
	}
}

func (d Dialect) AutoIncrementPK() string {
	switch d {
	case DialectPostgres:
		return "SERIAL PRIMARY KEY"
	case DialectSQLite:
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	default:
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	}
}

func (d Dialect) BigAutoIncrementPK() string {
	switch d {
	case DialectPostgres:
		return "BIGSERIAL PRIMARY KEY"
	case DialectSQLite:
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	default:
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	}
}

func (d Dialect) JSONType() string {
	switch d {
	case DialectPostgres:
		return "JSONB"
	case DialectSQLite:
		return "TEXT"
	default:
		return "TEXT"
	}
}

func (d Dialect) BooleanTrue() string {
	switch d {
	case DialectPostgres:
		return "TRUE"
	case DialectSQLite:
		return "1"
	default:
		return "1"
	}
}

func (d Dialect) BooleanFalse() string {
	switch d {
	case DialectPostgres:
		return "FALSE"
	case DialectSQLite:
		return "0"
	default:
		return "0"
	}
}

func (d Dialect) CurrentTimestamp() string {
	switch d {
	case DialectPostgres:
		return "CURRENT_TIMESTAMP"
	case DialectSQLite:
		return "CURRENT_TIMESTAMP"
	default:
		return "CURRENT_TIMESTAMP"
	}
}
