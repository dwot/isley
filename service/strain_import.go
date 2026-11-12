package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// CannadbStrain is a strain from CannaDB, already mapped to Isley's fields.
// URI (its AT-URI) identifies it: importing it again updates the strain it
// made rather than adding another.
type CannadbStrain struct {
	URI       string
	IndexedAt string

	Name             string
	IndicaPct        int
	SativaPct        int
	Autoflower       bool
	CycleTimeDays    int
	ShortDescription string
	Description      string
	URL              string

	Breeder CannadbBreeder
	// ParentNames replace the strain's lineage, as names only.
	ParentNames []string
}

// CannadbBreeder is the strain's breeder. URI is empty when CannaDB only
// knows the breeder's name.
type CannadbBreeder struct {
	Name      string
	URI       string
	IndexedAt string
}

// unknownBreeder names a breeder CannaDB doesn't give.
const unknownBreeder = "Unknown Breeder"

// ImportCannadb adds a strain from CannaDB, or updates the one an earlier
// import made, with its breeder and parents, in one transaction. The
// breeder is matched by its CannaDB link, then by name (ignoring case), and
// added if neither matches.
func (s *StrainService) ImportCannadb(ctx context.Context, in CannadbStrain) (Strain, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.URI = strings.TrimSpace(in.URI)
	var v Validation
	checkName(&v, "name", in.Name)
	if !strings.HasPrefix(in.URI, "at://") {
		v.Add("uri", "must be a CannaDB at:// URI")
	}
	if in.IndicaPct < 0 || in.SativaPct < 0 || in.IndicaPct+in.SativaPct != 100 {
		v.Add("indica_pct", "indica and sativa must add up to 100")
	}
	if err := v.Err(); err != nil {
		return Strain{}, err
	}

	var id int
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		breederID, err := upsertCannadbBreeder(ctx, tx, in.Breeder)
		if err != nil {
			return err
		}
		if id, err = upsertCannadbStrain(ctx, tx, breederID, in); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM strain_lineage WHERE strain_id = $1", id); err != nil {
			return fmt.Errorf("clear lineage: %w", err)
		}
		for _, name := range in.ParentNames {
			if name = strings.TrimSpace(name); name == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, NULL)",
				id, name); err != nil {
				return fmt.Errorf("add parent %q: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		return Strain{}, err
	}
	if err := s.RefreshCaches(ctx); err != nil {
		return Strain{}, err
	}
	return s.Get(ctx, id)
}

func upsertCannadbBreeder(ctx context.Context, tx *sql.Tx, b CannadbBreeder) (int, error) {
	name := strings.TrimSpace(b.Name)
	if name == "" {
		name = unknownBreeder
	}
	var id int
	if b.URI != "" {
		err := tx.QueryRowContext(ctx, "SELECT id FROM breeder WHERE cannadb_uri = $1", b.URI).Scan(&id)
		switch {
		case err == nil:
			if _, err := tx.ExecContext(ctx, "UPDATE breeder SET cannadb_indexed_at = $1 WHERE id = $2",
				nullable(b.IndexedAt), id); err != nil {
				return 0, fmt.Errorf("update breeder %d: %w", id, err)
			}
			// Take CannaDB's spelling of the name, unless another breeder
			// already has it (names are unique, ignoring case).
			if _, err := tx.ExecContext(ctx, `UPDATE breeder SET name = $1 WHERE id = $2
				AND NOT EXISTS (SELECT 1 FROM breeder o WHERE LOWER(o.name) = LOWER($1) AND o.id <> $2)`,
				name, id); err != nil {
				return 0, fmt.Errorf("rename breeder %d: %w", id, err)
			}
			return id, nil
		case !errors.Is(err, sql.ErrNoRows):
			return 0, fmt.Errorf("find breeder by CannaDB link: %w", err)
		}
	}

	// One the user added by hand: link it.
	err := tx.QueryRowContext(ctx, "SELECT id FROM breeder WHERE LOWER(name) = LOWER($1)", name).Scan(&id)
	switch {
	case err == nil:
		if b.URI != "" {
			if _, err := tx.ExecContext(ctx,
				"UPDATE breeder SET cannadb_uri = $1, cannadb_indexed_at = $2 WHERE id = $3",
				b.URI, nullable(b.IndexedAt), id); err != nil {
				return 0, fmt.Errorf("link breeder %d: %w", id, err)
			}
		}
		return id, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("find breeder by name: %w", err)
	}

	err = tx.QueryRowContext(ctx,
		"INSERT INTO breeder (name, cannadb_uri, cannadb_indexed_at) VALUES ($1, $2, $3) RETURNING id",
		name, nullable(b.URI), nullable(b.IndexedAt)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("add breeder: %w", err)
	}
	return id, nil
}

func upsertCannadbStrain(ctx context.Context, tx *sql.Tx, breederID int, in CannadbStrain) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, "SELECT id FROM strain WHERE cannadb_uri = $1", in.URI).Scan(&id)
	switch {
	case err == nil:
		_, err = tx.ExecContext(ctx, `
			UPDATE strain
			SET name = $1, breeder_id = $2, indica = $3, sativa = $4, autoflower = $5,
			    description = $6, short_desc = $7, cycle_time = $8, url = $9, cannadb_indexed_at = $10
			WHERE id = $11`,
			in.Name, breederID, in.IndicaPct, in.SativaPct, boolInt(in.Autoflower),
			in.Description, in.ShortDescription, in.CycleTimeDays, in.URL, nullable(in.IndexedAt), id)
		if err != nil {
			return 0, fmt.Errorf("update strain %d: %w", id, err)
		}
		return id, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("find strain by CannaDB link: %w", err)
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO strain (name, breeder_id, indica, sativa, autoflower, seed_count,
		                    description, short_desc, cycle_time, url, cannadb_uri, cannadb_indexed_at)
		VALUES ($1, $2, $3, $4, $5, 0, $6, $7, $8, $9, $10, $11) RETURNING id`,
		in.Name, breederID, in.IndicaPct, in.SativaPct, boolInt(in.Autoflower),
		in.Description, in.ShortDescription, in.CycleTimeDays, in.URL, in.URI, nullable(in.IndexedAt)).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("add strain: %w", err)
	}
	return id, nil
}

// nullable stores "" as NULL, which the partial unique indexes on
// cannadb_uri need.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
