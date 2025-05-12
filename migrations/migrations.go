package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
)

var migrationsFS embed.FS

func ApplyMigrations(db *sql.DB) error {
	files, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return getMigrationNumber(files[i].Name()) < getMigrationNumber(files[j].Name())
	})

	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".up.sql") {
			continue
		}

		migrationSQL, err := fs.ReadFile(migrationsFS, file.Name())
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", file.Name(), err)
		}

		log.Printf("Applying migration: %s", file.Name())
		if _, err := db.Exec(string(migrationSQL)); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", file.Name(), err)
		}
	}

	return nil
}

func getMigrationNumber(filename string) int {
	base := path.Base(filename)
	parts := strings.Split(base, "_")
	if len(parts) < 1 {
		return 0
	}
	num, _ := strconv.Atoi(parts[0])
	return num
}
