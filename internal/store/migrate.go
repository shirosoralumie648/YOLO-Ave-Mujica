package store

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(databaseURL, sourceURL, command string, forceVersion int) error {
	if databaseURL == "" {
		return errors.New("database url is required")
	}
	if sourceURL == "" {
		sourceURL = "file://migrations"
	}

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = m.Close()
	}()

	switch command {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
	case "down":
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
	case "version":
		_, _, err := m.Version()
		if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
			return err
		}
	case "force":
		return m.Force(forceVersion)
	default:
		return fmt.Errorf("unsupported migration command %q", command)
	}

	return nil
}
