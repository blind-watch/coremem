package migrations

import "embed"

//go:embed *.sql
var files embed.FS

type Migration struct {
	Name string
	SQL  string
}

func All() ([]Migration, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	out := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < 4 || entry.Name()[len(entry.Name())-4:] != ".sql" {
			continue
		}
		b, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Name: entry.Name(), SQL: string(b)})
	}
	return out, nil
}
