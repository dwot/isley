package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"isley/config"
	"isley/model/types"
	"isley/utils"
)

// Strain error codes.
const (
	CodeStrainNotFound       = "strain_not_found"
	CodeStrainInUse          = "strain_in_use"
	CodeNotEnoughSeeds       = "not_enough_seeds"
	CodeLineageEntryNotFound = "lineage_entry_not_found"
	CodeLineageCycle         = "lineage_cycle"
)

const (
	maxStrainDescription      = utils.MaxNotesLength
	maxStrainShortDescription = utils.MaxDescriptionLength
	// MaxAncestryDepth bounds how many generations Ancestry walks.
	MaxAncestryDepth = 10
)

// Strain is a cultivar from a breeder.
type Strain struct {
	ID        int
	Name      string
	Breeder   Ref
	IndicaPct int
	SativaPct int
	// Autoflower strains flower on age rather than light.
	Autoflower bool
	// Feminized seeds grow only female plants; regular seeds give both.
	Feminized bool
	SeedCount int
	// CycleTimeDays is seed to harvest for an autoflower, or the flowering
	// time for a photoperiod strain; nil when unknown.
	CycleTimeDays    *int
	URL              string
	ShortDescription string
	Description      string
	// CannadbURI is set on strains imported from CannaDB.
	CannadbURI string
	// Parents are the strain's direct parents, by name.
	Parents []LineageEntry
}

// LineageEntry is one parent of a strain: a name, linked to a strain in
// this instance when there is one.
type LineageEntry struct {
	ID         int
	ParentName string
	Parent     *Ref
}

// AncestryNode is a parent with its own parents, as far as they are known.
type AncestryNode struct {
	LineageEntry
	Parents []AncestryNode
}

// Descendant is a strain with a given strain as a parent.
type Descendant struct {
	Ref
	Breeder Ref
	// OtherParents are the descendant's other parents' names.
	OtherParents []string
}

// ParentInput names a parent. With StrainID set, Name defaults to that
// strain's name.
type ParentInput struct {
	Name     string
	StrainID *int
}

// StrainInput is a new strain. BreederID picks an existing breeder; when
// it is 0, BreederName names one, created if there is none by that name.
type StrainInput struct {
	Name        string
	BreederID   int
	BreederName string
	IndicaPct   int
	SativaPct   int
	Autoflower  bool
	// Feminized nil means feminized, the default for a new strain.
	Feminized        *bool
	SeedCount        int
	CycleTimeDays    *int
	URL              string
	ShortDescription string
	Description      string
}

// StrainUpdate changes the fields that are set. BreederName (when not
// empty) moves the strain to the breeder of that name, created if needed,
// and wins over BreederID.
type StrainUpdate struct {
	Name             Patch[string]
	BreederID        Patch[int]
	BreederName      string
	IndicaPct        Patch[int]
	SativaPct        Patch[int]
	Autoflower       Patch[bool]
	Feminized        Patch[bool]
	SeedCount        Patch[int]
	CycleTimeDays    Patch[int]
	URL              Patch[string]
	ShortDescription Patch[string]
	Description      Patch[string]
}

// StrainFilter selects strains. Results are by name.
type StrainFilter struct {
	BreederID int
	// InStock, when set, keeps strains with (true) or without (false) seeds.
	InStock *bool
	// Feminized, when set, keeps feminized (true) or regular (false) strains.
	Feminized *bool
	// Query matches part of the name, ignoring case.
	Query  string
	Offset int
	// Limit 0 means all.
	Limit int
}

// StrainService owns strains and their lineage.
type StrainService struct {
	deps Deps
}

func (s *StrainService) breeders() *BreederService { return &BreederService{deps: s.deps} }

func strainNotFound(id int) error {
	return NotFound(CodeStrainNotFound, fmt.Sprintf("strain %d does not exist", id))
}

const strainSelect = `
	SELECT s.id, s.name, s.breeder_id, COALESCE(b.name, ''), s.indica, s.sativa, s.autoflower, s.feminized, s.seed_count,
	       s.cycle_time, COALESCE(s.url, ''), COALESCE(s.short_desc, ''), s.description,
	       COALESCE(s.cannadb_uri, '')
	FROM strain s LEFT JOIN breeder b ON b.id = s.breeder_id`

// List returns the strains matching f, with the total that match.
func (s *StrainService) List(ctx context.Context, f StrainFilter) ([]Strain, int, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.Replace(cond, "?", fmt.Sprintf("$%d", len(args)), 1))
	}
	if f.BreederID > 0 {
		add("s.breeder_id = ?", f.BreederID)
	}
	if f.InStock != nil {
		if *f.InStock {
			where = append(where, "s.seed_count > 0")
		} else {
			where = append(where, "s.seed_count <= 0")
		}
	}
	if f.Feminized != nil {
		add("s.feminized = ?", boolInt(*f.Feminized))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		add("LOWER(s.name) LIKE ?", "%"+strings.ToLower(q)+"%")
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := s.deps.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM strain s"+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count strains: %w", err)
	}
	q := strainSelect + cond + " ORDER BY LOWER(s.name), s.id"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	out, err := s.query(ctx, q, args...)
	return out, total, err
}

// Get returns one strain with its parents.
func (s *StrainService) Get(ctx context.Context, id int) (Strain, error) {
	out, err := s.query(ctx, strainSelect+" WHERE s.id = $1", id)
	if err != nil {
		return Strain{}, err
	}
	if len(out) == 0 {
		return Strain{}, strainNotFound(id)
	}
	return out[0], nil
}

func (s *StrainService) query(ctx context.Context, q string, args ...any) ([]Strain, error) {
	rows, err := s.deps.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list strains: %w", err)
	}
	defer rows.Close()
	out := []Strain{}
	for rows.Next() {
		var (
			st    Strain
			cycle sql.NullInt64
		)
		if err := rows.Scan(&st.ID, &st.Name, &st.Breeder.ID, &st.Breeder.Name, &st.IndicaPct, &st.SativaPct,
			&st.Autoflower, &st.Feminized, &st.SeedCount, &cycle, &st.URL, &st.ShortDescription, &st.Description,
			&st.CannadbURI); err != nil {
			return nil, fmt.Errorf("scan strain: %w", err)
		}
		if cycle.Valid && cycle.Int64 > 0 {
			days := int(cycle.Int64)
			st.CycleTimeDays = &days
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list strains: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]int, len(out))
	for i, st := range out {
		ids[i] = st.ID
	}
	parents, err := lineageOf(ctx, s.deps.DB, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Parents = parents[out[i].ID]
		if out[i].Parents == nil {
			out[i].Parents = []LineageEntry{}
		}
	}
	return out, nil
}

// lineageOf returns the direct parents of each strain, by name.
func lineageOf(ctx context.Context, q querier, strainIDs []int) (map[int][]LineageEntry, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`
		SELECT sl.strain_id, sl.id, sl.parent_name, p.id, p.name
		FROM strain_lineage sl LEFT JOIN strain p ON p.id = sl.parent_strain_id
		WHERE sl.strain_id IN (%s)
		ORDER BY LOWER(sl.parent_name), sl.id`, Placeholders(1, len(strainIDs))), toAny(strainIDs)...)
	if err != nil {
		return nil, fmt.Errorf("strain lineage: %w", err)
	}
	defer rows.Close()
	out := map[int][]LineageEntry{}
	for rows.Next() {
		var (
			strainID int
			e        LineageEntry
			pid      sql.NullInt64
			pname    sql.NullString
		)
		if err := rows.Scan(&strainID, &e.ID, &e.ParentName, &pid, &pname); err != nil {
			return nil, fmt.Errorf("scan lineage: %w", err)
		}
		if pid.Valid {
			e.Parent = &Ref{ID: int(pid.Int64), Name: pname.String}
		}
		out[strainID] = append(out[strainID], e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("strain lineage: %w", err)
	}
	return out, nil
}

// validate checks a strain's fields, normalising them in place.
func (s *StrainService) validate(v *Validation, st *Strain) {
	st.Name = strings.TrimSpace(st.Name)
	checkName(v, "name", st.Name)
	for field, pct := range map[string]int{"indica_pct": st.IndicaPct, "sativa_pct": st.SativaPct} {
		if pct < 0 || pct > 100 {
			v.Add(field, "must be from 0 to 100")
		}
	}
	if st.IndicaPct+st.SativaPct != 100 {
		v.Add("sativa_pct", "indica_pct and sativa_pct must add up to 100")
	}
	if st.SeedCount < 0 {
		v.Add("seed_count", "must not be negative")
	}
	if st.CycleTimeDays != nil && *st.CycleTimeDays < 0 {
		v.Add("cycle_time_days", "must not be negative")
	}
	if st.CycleTimeDays != nil && *st.CycleTimeDays == 0 {
		st.CycleTimeDays = nil
	}
	// Schemeless addresses (www.example.com/x) were accepted once; they
	// are kept usable by assuming https.
	st.URL = utils.NormalizeWebURL(st.URL)
	if err := utils.ValidateWebURL("url", st.URL); err != nil {
		v.Add("url", strings.TrimPrefix(err.Error(), "url "))
	}
	if n := utf8.RuneCountInString(st.ShortDescription); n > maxStrainShortDescription {
		v.Add("short_description", fmt.Sprintf("must be at most %d characters", maxStrainShortDescription))
	}
	if n := utf8.RuneCountInString(st.Description); n > maxStrainDescription {
		v.Add("description", fmt.Sprintf("must be at most %d characters", maxStrainDescription))
	}
}

// resolveBreeder returns the breeder id to use: name (found or created)
// when given, else id, which must exist. Field problems go to v.
func (s *StrainService) resolveBreeder(ctx context.Context, tx *sql.Tx, v *Validation, id int, name string) (int, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		if len(name) > maxNameLength {
			v.Add("breeder_name", fmt.Sprintf("must be at most %d characters", maxNameLength))
			return 0, nil
		}
		return s.breeders().findOrCreate(ctx, tx, name)
	}
	if id <= 0 {
		v.Add("breeder_id", "is required (or give breeder_name)")
		return 0, nil
	}
	found, err := existingIDs(ctx, tx, "breeder", []int{id})
	if err != nil {
		return 0, err
	}
	if !found[id] {
		v.Add("breeder_id", "does not exist")
	}
	return id, nil
}

// Create adds a strain.
func (s *StrainService) Create(ctx context.Context, in StrainInput) (Strain, error) {
	st := Strain{
		Name: in.Name, IndicaPct: in.IndicaPct, SativaPct: in.SativaPct, Autoflower: in.Autoflower,
		Feminized: in.Feminized == nil || *in.Feminized, SeedCount: in.SeedCount, CycleTimeDays: in.CycleTimeDays, URL: in.URL,
		ShortDescription: in.ShortDescription, Description: in.Description,
	}
	var v Validation
	s.validate(&v, &st)
	if in.BreederID <= 0 && strings.TrimSpace(in.BreederName) == "" {
		v.Add("breeder_id", "is required (or give breeder_name)")
	}
	if err := v.Err(); err != nil {
		return Strain{}, err
	}
	var id int
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		bid, err := s.resolveBreeder(ctx, tx, &v, in.BreederID, in.BreederName)
		if err != nil {
			return err
		}
		if err := v.Err(); err != nil {
			return err
		}
		st.Breeder.ID = bid
		return tx.QueryRowContext(ctx, `
			INSERT INTO strain (name, breeder_id, indica, sativa, autoflower, feminized, seed_count, cycle_time, url, short_desc, description)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			st.Name, st.Breeder.ID, st.IndicaPct, st.SativaPct, boolInt(st.Autoflower), boolInt(st.Feminized), st.SeedCount,
			st.CycleTimeDays, st.URL, st.ShortDescription, st.Description).Scan(&id)
	})
	if err != nil {
		return Strain{}, err
	}
	return s.afterWrite(ctx, id)
}

// Update changes a strain.
func (s *StrainService) Update(ctx context.Context, id int, upd StrainUpdate) (Strain, error) {
	st, err := s.Get(ctx, id)
	if err != nil {
		return Strain{}, err
	}
	setString := func(dst *string, p Patch[string]) {
		if p.Set {
			*dst = ""
			if p.Value != nil {
				*dst = *p.Value
			}
		}
	}
	setInt := func(dst *int, p Patch[int]) {
		if p.Set {
			*dst = 0
			if p.Value != nil {
				*dst = *p.Value
			}
		}
	}
	setString(&st.Name, upd.Name)
	setInt(&st.IndicaPct, upd.IndicaPct)
	setInt(&st.SativaPct, upd.SativaPct)
	setInt(&st.SeedCount, upd.SeedCount)
	if upd.Autoflower.Set {
		st.Autoflower = upd.Autoflower.Value != nil && *upd.Autoflower.Value
	}
	if upd.Feminized.Set {
		// Cleared means the default, feminized.
		st.Feminized = upd.Feminized.Value == nil || *upd.Feminized.Value
	}
	if upd.CycleTimeDays.Set {
		st.CycleTimeDays = upd.CycleTimeDays.Value
	}
	setString(&st.URL, upd.URL)
	setString(&st.ShortDescription, upd.ShortDescription)
	setString(&st.Description, upd.Description)
	var v Validation
	s.validate(&v, &st)
	if err := v.Err(); err != nil {
		return Strain{}, err
	}
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		if upd.BreederName != "" || upd.BreederID.Set {
			bid := 0
			if upd.BreederID.Value != nil {
				bid = *upd.BreederID.Value
			}
			resolved, err := s.resolveBreeder(ctx, tx, &v, bid, upd.BreederName)
			if err != nil {
				return err
			}
			if err := v.Err(); err != nil {
				return err
			}
			st.Breeder.ID = resolved
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE strain SET name = $1, breeder_id = $2, indica = $3, sativa = $4, autoflower = $5,
			       feminized = $6, seed_count = $7, cycle_time = $8, url = $9, short_desc = $10, description = $11
			WHERE id = $12`,
			st.Name, st.Breeder.ID, st.IndicaPct, st.SativaPct, boolInt(st.Autoflower), boolInt(st.Feminized), st.SeedCount,
			st.CycleTimeDays, st.URL, st.ShortDescription, st.Description, id)
		if err != nil {
			return fmt.Errorf("update strain %d: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return Strain{}, err
	}
	return s.afterWrite(ctx, id)
}

func (s *StrainService) afterWrite(ctx context.Context, id int) (Strain, error) {
	if err := refreshBreeders(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Strain{}, err
	}
	if err := refreshStrains(ctx, s.deps.DB, s.deps.Store); err != nil {
		return Strain{}, err
	}
	return s.Get(ctx, id)
}

// AdjustSeeds adds delta (negative to take seeds out) to the seed count in
// one statement, so two clients adjusting at once both count. It refuses
// to go below zero.
func (s *StrainService) AdjustSeeds(ctx context.Context, id, delta int) (Strain, error) {
	if delta == 0 {
		return Strain{}, Invalid(map[string]string{"delta": "must not be 0"})
	}
	res, err := s.deps.DB.ExecContext(ctx,
		"UPDATE strain SET seed_count = seed_count + $1 WHERE id = $2 AND seed_count + $1 >= 0", delta, id)
	if err != nil {
		return Strain{}, fmt.Errorf("adjust seeds of strain %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return Strain{}, fmt.Errorf("rows affected: %w", err)
	} else if n == 0 {
		st, err := s.Get(ctx, id)
		if err != nil {
			return Strain{}, err
		}
		return Strain{}, Conflict(CodeNotEnoughSeeds, fmt.Sprintf(
			"strain %d has %s; can't take out %d", id, count(st.SeedCount, "seed"), -delta))
	}
	return s.afterWrite(ctx, id)
}

// Delete deletes a strain no plant uses. Strains that named it as a
// parent keep the name, unlinked.
func (s *StrainService) Delete(ctx context.Context, id int) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	var plants int
	if err := s.deps.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM plant WHERE strain_id = $1", id).Scan(&plants); err != nil {
		return fmt.Errorf("count strain %d plants: %w", id, err)
	}
	if plants > 0 {
		return Conflict(CodeStrainInUse, fmt.Sprintf(
			"strain %d has %s; move or delete them first", id, count(plants, "plant")))
	}
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		for _, q := range []string{
			"DELETE FROM strain_lineage WHERE strain_id = $1",
			"UPDATE strain_lineage SET parent_strain_id = NULL WHERE parent_strain_id = $1",
			"DELETE FROM strain WHERE id = $1",
		} {
			if _, err := tx.ExecContext(ctx, q, id); err != nil {
				return fmt.Errorf("delete strain %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return refreshStrains(ctx, s.deps.DB, s.deps.Store)
}

// Lineage returns a strain's direct parents.
func (s *StrainService) Lineage(ctx context.Context, id int) ([]LineageEntry, error) {
	st, err := s.Get(ctx, id)
	return st.Parents, err
}

// Ancestry returns a strain's parents with theirs, up to MaxAncestryDepth
// generations. A strain that turns up again in its own line is shown but
// not followed.
func (s *StrainService) Ancestry(ctx context.Context, id int) ([]AncestryNode, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.ancestry(ctx, id, map[int]bool{id: true}, 1)
}

func (s *StrainService) ancestry(ctx context.Context, id int, line map[int]bool, depth int) ([]AncestryNode, error) {
	parents, err := lineageOf(ctx, s.deps.DB, []int{id})
	if err != nil {
		return nil, err
	}
	out := make([]AncestryNode, 0, len(parents[id]))
	for _, e := range parents[id] {
		node := AncestryNode{LineageEntry: e, Parents: []AncestryNode{}}
		if e.Parent != nil && depth < MaxAncestryDepth && !line[e.Parent.ID] {
			line[e.Parent.ID] = true
			if node.Parents, err = s.ancestry(ctx, e.Parent.ID, line, depth+1); err != nil {
				return nil, err
			}
			delete(line, e.Parent.ID)
		}
		out = append(out, node)
	}
	return out, nil
}

// checkParent validates one parent of strainID, filling in its name from
// a linked strain. field prefixes problem names.
func (s *StrainService) checkParent(ctx context.Context, q querier, v *Validation, strainID int, field string, p *ParentInput) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.StrainID != nil {
		pid := *p.StrainID
		var name string
		err := q.QueryRowContext(ctx, "SELECT name FROM strain WHERE id = $1", pid).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			v.Add(field+"strain_id", "does not exist")
			return nil
		}
		if err != nil {
			return fmt.Errorf("parent strain: %w", err)
		}
		if p.Name == "" {
			p.Name = name
		}
		if pid == strainID {
			return Conflict(CodeLineageCycle, "a strain can't be its own parent")
		}
		ancestors, err := s.ancestorsOf(ctx, q, pid)
		if err != nil {
			return err
		}
		if ancestors[strainID] {
			return Conflict(CodeLineageCycle, fmt.Sprintf(
				"strain %d descends from strain %d, so it can't be its parent", pid, strainID))
		}
	}
	checkName(v, field+"name", p.Name)
	return nil
}

// ancestorsOf returns every linked ancestor of a strain.
func (s *StrainService) ancestorsOf(ctx context.Context, q querier, id int) (map[int]bool, error) {
	seen := map[int]bool{}
	next := []int{id}
	for len(next) > 0 {
		rows, err := q.QueryContext(ctx, fmt.Sprintf(
			"SELECT parent_strain_id FROM strain_lineage WHERE parent_strain_id IS NOT NULL AND strain_id IN (%s)",
			Placeholders(1, len(next))), toAny(next)...)
		if err != nil {
			return nil, fmt.Errorf("strain ancestors: %w", err)
		}
		next = next[:0]
		for rows.Next() {
			var pid int
			if err := rows.Scan(&pid); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan ancestor: %w", err)
			}
			if !seen[pid] {
				seen[pid] = true
				next = append(next, pid)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("strain ancestors: %w", err)
		}
	}
	return seen, nil
}

// SetLineage replaces a strain's parents.
func (s *StrainService) SetLineage(ctx context.Context, id int, parents []ParentInput) ([]LineageEntry, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		var v Validation
		for i := range parents {
			if err := s.checkParent(ctx, tx, &v, id, fmt.Sprintf("parents[%d].", i), &parents[i]); err != nil {
				return err
			}
		}
		if err := v.Err(); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM strain_lineage WHERE strain_id = $1", id); err != nil {
			return fmt.Errorf("clear lineage: %w", err)
		}
		for _, p := range parents {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, $3)",
				id, p.Name, p.StrainID); err != nil {
				return fmt.Errorf("add parent: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Lineage(ctx, id)
}

// AddParent adds one parent to a strain.
func (s *StrainService) AddParent(ctx context.Context, id int, p ParentInput) (LineageEntry, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return LineageEntry{}, err
	}
	var entryID int
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		var v Validation
		if err := s.checkParent(ctx, tx, &v, id, "parent_", &p); err != nil {
			return err
		}
		if err := v.Err(); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx,
			"INSERT INTO strain_lineage (strain_id, parent_name, parent_strain_id) VALUES ($1, $2, $3) RETURNING id",
			id, p.Name, p.StrainID).Scan(&entryID)
	})
	if err != nil {
		return LineageEntry{}, err
	}
	return s.lineageEntry(ctx, entryID)
}

func (s *StrainService) entryStrain(ctx context.Context, entryID int) (int, error) {
	var strainID int
	err := s.deps.DB.QueryRowContext(ctx, "SELECT strain_id FROM strain_lineage WHERE id = $1", entryID).Scan(&strainID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, NotFound(CodeLineageEntryNotFound, fmt.Sprintf("lineage entry %d does not exist", entryID))
	}
	if err != nil {
		return 0, fmt.Errorf("lineage entry %d: %w", entryID, err)
	}
	return strainID, nil
}

func (s *StrainService) lineageEntry(ctx context.Context, entryID int) (LineageEntry, error) {
	strainID, err := s.entryStrain(ctx, entryID)
	if err != nil {
		return LineageEntry{}, err
	}
	all, err := lineageOf(ctx, s.deps.DB, []int{strainID})
	if err != nil {
		return LineageEntry{}, err
	}
	for _, e := range all[strainID] {
		if e.ID == entryID {
			return e, nil
		}
	}
	return LineageEntry{}, NotFound(CodeLineageEntryNotFound, fmt.Sprintf("lineage entry %d does not exist", entryID))
}

// UpdateParent changes one lineage entry.
func (s *StrainService) UpdateParent(ctx context.Context, entryID int, p ParentInput) (LineageEntry, error) {
	strainID, err := s.entryStrain(ctx, entryID)
	if err != nil {
		return LineageEntry{}, err
	}
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		var v Validation
		if err := s.checkParent(ctx, tx, &v, strainID, "parent_", &p); err != nil {
			return err
		}
		if err := v.Err(); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"UPDATE strain_lineage SET parent_name = $1, parent_strain_id = $2 WHERE id = $3",
			p.Name, p.StrainID, entryID)
		return err
	})
	if err != nil {
		return LineageEntry{}, err
	}
	return s.lineageEntry(ctx, entryID)
}

// DeleteParent removes one lineage entry.
func (s *StrainService) DeleteParent(ctx context.Context, entryID int) error {
	res, err := s.deps.DB.ExecContext(ctx, "DELETE FROM strain_lineage WHERE id = $1", entryID)
	if err != nil {
		return fmt.Errorf("delete lineage entry %d: %w", entryID, err)
	}
	return requireRow(res, NotFound(CodeLineageEntryNotFound, fmt.Sprintf("lineage entry %d does not exist", entryID)))
}

// Descendants returns the strains that name this one as a parent, by name.
func (s *StrainService) Descendants(ctx context.Context, id int) ([]Descendant, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT DISTINCT s.id, s.name, b.id, b.name
		FROM strain_lineage sl
		JOIN strain s ON s.id = sl.strain_id
		JOIN breeder b ON b.id = s.breeder_id
		WHERE sl.parent_strain_id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("strain descendants: %w", err)
	}
	defer rows.Close()
	out := []Descendant{}
	for rows.Next() {
		var d Descendant
		if err := rows.Scan(&d.ID, &d.Name, &d.Breeder.ID, &d.Breeder.Name); err != nil {
			return nil, fmt.Errorf("scan descendant: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("strain descendants: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]int, len(out))
	for i, d := range out {
		ids[i] = d.ID
	}
	parents, err := lineageOf(ctx, s.deps.DB, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].OtherParents = []string{}
		for _, e := range parents[out[i].ID] {
			if e.Parent == nil || e.Parent.ID != id {
				out[i].OtherParents = append(out[i].OtherParents, e.ParentName)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// count is "1 seed", "2 seeds".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// RefreshCaches reloads the strain and breeder caches from the database,
// after writes that bypass the service (CannaDB import, restore).
func (s *StrainService) RefreshCaches(ctx context.Context) error {
	if err := refreshBreeders(ctx, s.deps.DB, s.deps.Store); err != nil {
		return err
	}
	return refreshStrains(ctx, s.deps.DB, s.deps.Store)
}

// refreshStrains reloads the strain cache the web pages read.
func refreshStrains(ctx context.Context, q querier, store *config.Store) error {
	rows, err := q.QueryContext(ctx, `
		SELECT s.id, s.name, s.breeder_id, COALESCE(b.name, ''), s.indica, s.sativa, s.autoflower, s.feminized,
		       s.description, COALESCE(s.short_desc, ''), s.seed_count
		FROM strain s LEFT JOIN breeder b ON s.breeder_id = b.id
		ORDER BY s.name`)
	if err != nil {
		return fmt.Errorf("list strains: %w", err)
	}
	defer rows.Close()
	var out []types.Strain
	for rows.Next() {
		var st types.Strain
		if err := rows.Scan(&st.ID, &st.Name, &st.BreederID, &st.Breeder, &st.Indica, &st.Sativa,
			&st.Autoflower, &st.Feminized, &st.Description, &st.ShortDescription, &st.SeedCount); err != nil {
			return fmt.Errorf("scan strain: %w", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("list strains: %w", err)
	}
	store.SetStrains(out)
	return nil
}
