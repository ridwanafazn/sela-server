// Package migrations embeds the versioned SQL migrations so the API and the
// migration runner can ship as a single container image.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// FS contains every migration file.
//
//go:embed *.sql
var FS embed.FS

// LatestVersion returns the highest migration version present in the binary.
func LatestVersion() (uint, error) {
	names, err := fs.Glob(FS, "*.up.sql")
	if err != nil {
		return 0, fmt.Errorf("list migrations: %w", err)
	}

	var latest uint
	for _, name := range names {
		version, err := versionFromFilename(name)
		if err != nil {
			return 0, err
		}
		if version > latest {
			latest = version
		}
	}

	if latest == 0 {
		return 0, fmt.Errorf("no migrations found")
	}

	return latest, nil
}

func versionFromFilename(name string) (uint, error) {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return 0, fmt.Errorf("migration %q does not follow the version_name pattern", name)
	}

	value, err := strconv.ParseUint(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("migration %q has an invalid version: %w", name, err)
	}

	return uint(value), nil
}
