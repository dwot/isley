package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"isley/config"
	"isley/model/types"
)

// Breeder error codes.
const (
	CodeBreederNotFound  = "breeder_not_found"
	CodeBreederNotEmpty  = "breeder_not_empty"
	CodeBreederNameTaken = "breeder_name_taken"
)

// Breeder is a seed company.
type Breeder struct {
	ID          int
	Name        string
	StrainCount int
}

// BreederService owns breeders.
type BreederService struct {
	deps Deps
}

func breederNotFound(id int) error {
	return NotFound(CodeBreederNotFound, fmt.Sprintf("breeder %d does not exist", id))
}

const breederSelect = `
	SELECT b.id, b.name, (SELECT COUNT(*) FROM strain s WHERE s.breeder_id = b.id)
	FROM breeder b`

func scanBreeders(rows *sql.Rows) ([]Breeder, error) {
	defer rows.Close()
	out := []Breeder{}
	for rows.Next() {
		var b Breeder
		if err := rows.Scan(&b.ID, &b.Name, &b.StrainCount); err != nil {
			return nil, fmt.Errorf("scan breeder: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list breeders: %w", err)
	}
	return out, nil
}

// List returns breeders by name, a page at a time (limit 0 means all),
// with the total count.
func (s *BreederService) List(ctx context.Context, offset, limit int) ([]Breeder, int, error) {
	var total int
	if err := s.deps.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM breeder").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count breeders: %w", err)
	}
	q := breederSelect + " ORDER BY LOWER(b.name), b.id"
	args := []any{}
	if limit > 0 {
		q += " LIMIT $1 OFFSET $2"
		args = append(args, limit, offset)
	}
	rows, err := s.deps.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list breeders: %w", err)
	}
	out, err := scanBreeders(rows)
	return out, total, err
}

// Get returns one breeder.
func (s *BreederService) Get(ctx context.Context, id int) (Breeder, error) {
	rows, err := s.deps.DB.QueryContext(ctx, breederSelect+" WHERE b.id = $1", id)
	if err != nil {
		return Breeder{}, fmt.Errorf("get breeder %d: %w", id, err)
	}
	out, err := scanBreeders(rows)
	if err != nil {
		return Breeder{}, err
	}
	if len(out) == 0 {
		return Breeder{}, breederNotFound(id)
	}
	return out[0], nil
}

// byName finds a breeder by name, ignoring case and surrounding space.
func (s *BreederService) byName(ctx context.Context, q querier, name string) (int, bool, error) {
	var id int
	err := q.QueryRowContext(ctx,
		"SELECT id FROM breeder WHERE LOWER(name) = LOWER($1) ORDER BY id LIMIT 1", strings.TrimSpace(name)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find breeder: %w", err)
	}
	return id, true, nil
}

// findOrCreate returns the breeder with this name, creating it if there
// is none. The caller checks the name and refreshes the caches.
func (s *BreederService) findOrCreate(ctx context.Context, q querier, name string) (int, error) {
	id, found, err := s.byName(ctx, q, name)
	if err != nil || found {
		return id, err
	}
	if err := q.QueryRowContext(ctx, "INSERT INTO breeder (name) VALUES ($1) RETURNING id", name).Scan(&id); err != nil {
		return 0, fmt.Errorf("add breeder: %w", err)
	}
	return id, nil
}

// Create adds a breeder. Names are unique, ignoring case.
func (s *BreederService) Create(ctx context.Context, name string) (Breeder, error) {
	name = strings.TrimSpace(name)
	var v Validation
	checkName(&v, "name", name)
	if err := v.Err(); err != nil {
		return Breeder{}, err
	}
	if err := s.checkNameFree(ctx, name, 0); err != nil {
		return Breeder{}, err
	}
	var id int
	if err := s.deps.DB.QueryRowContext(ctx, "INSERT INTO breeder (name) VALUES ($1) RETURNING id", name).Scan(&id); err != nil {
		return Breeder{}, fmt.Errorf("add breeder: %w", err)
	}
	if err := refreshBreeders(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Breeder{}, err
	}
	return s.Get(ctx, id)
}

func (s *BreederService) checkNameFree(ctx context.Context, name string, self int) error {
	id, found, err := s.byName(ctx, s.deps.DB, name)
	if err != nil {
		return err
	}
	if found && id != self {
		return Conflict(CodeBreederNameTaken, fmt.Sprintf("breeder %d is already called %q", id, name))
	}
	return nil
}

// Rename renames a breeder.
func (s *BreederService) Rename(ctx context.Context, id int, name string) (Breeder, error) {
	name = strings.TrimSpace(name)
	var v Validation
	checkName(&v, "name", name)
	if err := v.Err(); err != nil {
		return Breeder{}, err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return Breeder{}, err
	}
	if err := s.checkNameFree(ctx, name, id); err != nil {
		return Breeder{}, err
	}
	if _, err := s.deps.DB.ExecContext(ctx, "UPDATE breeder SET name = $1 WHERE id = $2", name, id); err != nil {
		return Breeder{}, fmt.Errorf("rename breeder %d: %w", id, err)
	}
	// The strain cache carries breeder names too.
	if err := refreshBreeders(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Breeder{}, err
	}
	if err := refreshStrains(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Breeder{}, err
	}
	return s.Get(ctx, id)
}

// Delete deletes a breeder that has no strains.
func (s *BreederService) Delete(ctx context.Context, id int) error {
	b, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if b.StrainCount > 0 {
		return Conflict(CodeBreederNotEmpty, fmt.Sprintf(
			"breeder %d still has %s; move or delete them first", id, count(b.StrainCount, "strain")))
	}
	if _, err := s.deps.DB.ExecContext(ctx, "DELETE FROM breeder WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete breeder %d: %w", id, err)
	}
	return refreshBreeders(ctx, s.deps.DB, s.deps.Store)
}

// DeleteWithEverything deletes a breeder, its strains and every plant of
// those strains (with their images). The settings page offers this; the
// API does not.
func (s *BreederService) DeleteWithEverything(ctx context.Context, id int) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT p.id FROM plant p JOIN strain s ON s.id = p.strain_id WHERE s.breeder_id = $1`, id)
	if err != nil {
		return fmt.Errorf("list breeder %d plants: %w", id, err)
	}
	var plants []int
	for rows.Next() {
		var pid int
		if err := rows.Scan(&pid); err != nil {
			rows.Close()
			return fmt.Errorf("scan plant: %w", err)
		}
		plants = append(plants, pid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list breeder %d plants: %w", id, err)
	}
	ps := &PlantService{deps: s.deps}
	for _, pid := range plants {
		if err := ps.Delete(ctx, pid); err != nil {
			return err
		}
	}
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		strains := "SELECT id FROM strain WHERE breeder_id = $1"
		for _, q := range []string{
			"DELETE FROM strain_lineage WHERE strain_id IN (" + strains + ")",
			"UPDATE strain_lineage SET parent_strain_id = NULL WHERE parent_strain_id IN (" + strains + ")",
			"DELETE FROM strain WHERE breeder_id = $1",
			"DELETE FROM breeder WHERE id = $1",
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return fmt.Errorf("delete breeder %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := refreshBreeders(ctx, s.deps.DB, s.deps.Store); err != nil {
		return err
	}
	return refreshStrains(ctx, s.deps.DB, s.deps.Store)
}

// refreshBreeders reloads the breeder cache the web pages read.
func refreshBreeders(ctx context.Context, q querier, store *config.Store) error {
	rows, err := q.QueryContext(ctx, "SELECT id, name FROM breeder ORDER BY id")
	if err != nil {
		return fmt.Errorf("list breeders: %w", err)
	}
	defer rows.Close()
	var out []types.Breeder
	for rows.Next() {
		var b types.Breeder
		if err := rows.Scan(&b.ID, &b.Name); err != nil {
			return fmt.Errorf("scan breeder: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list breeders: %w", err)
	}
	store.SetBreeders(out)
	return nil
}
