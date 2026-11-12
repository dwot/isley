package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"isley/chrono"
	"isley/config"
	"isley/model/types"
	"isley/utils"
)

// Smoke report error codes.
const (
	CodeSmokeReportNotFound    = "smoke_report_not_found"
	CodeSmokeCriterionNotFound = "smoke_criterion_not_found"
	CodePlantNotHarvested      = "plant_not_harvested"
)

const (
	// SmokeRatingMin and SmokeRatingMax bound a report's rating and each
	// criterion score.
	SmokeRatingMin = 1
	SmokeRatingMax = 5
	// MaxSmokeCriterionName bounds a criterion's name.
	MaxSmokeCriterionName = 50
	// MaxConsumptionMethod bounds the free-text consumption method.
	MaxConsumptionMethod = 100
)

// SmokeCriterion is a tasting axis (Aroma, Taste ...). Criteria that have
// been scored are archived (Active false) rather than deleted: a tasting
// can't be repeated the way a measurement can, so removing one must never
// destroy recorded scores.
type SmokeCriterion struct {
	ID        int
	Name      string
	SortOrder int
	Active    bool
}

// SmokeScore is one criterion's score in a report.
type SmokeScore struct {
	Criterion Ref
	Value     int
}

// SmokeReport is a post-harvest tasting note for a plant.
type SmokeReport struct {
	ID                int
	PlantID           int
	ReportDate        chrono.Date
	ConsumptionMethod string
	Rating            int
	Description       string
	// CureDays is the calendar days from the plant's harvest to the
	// report, or nil without a harvest day or when the report predates it.
	CureDays *int
	Scores   []SmokeScore
}

// SmokeScoreInput is one criterion's score.
type SmokeScoreInput struct {
	CriterionID int
	Value       int
}

// SmokeReportInput is a report's content. ReportDate defaults to today.
// Scores replace the report's scores on active criteria; scores on
// archived criteria are kept as they are.
type SmokeReportInput struct {
	ReportDate        chrono.Date
	ConsumptionMethod string
	Rating            int
	Description       string
	Scores            []SmokeScoreInput
}

// SmokeSummaryEntry is one harvest's place in a strain's summary: the
// rating of that plant's latest report.
type SmokeSummaryEntry struct {
	Plant      Ref
	Rating     int
	ReportDate chrono.Date
}

// SmokeSummary averages a strain's harvests, one entry per plant (its
// latest report), so a plant tasted ten times doesn't outweigh one tasted
// once and the average is the mean of the entries shown.
type SmokeSummary struct {
	Average float64
	Entries []SmokeSummaryEntry
}

// SmokeReportService owns smoke reports and their criteria.
type SmokeReportService struct {
	deps Deps
}

func smokeReportNotFound(id int) error {
	return NotFound(CodeSmokeReportNotFound, fmt.Sprintf("smoke report %d does not exist", id))
}

func smokeCriterionNotFound(id int) error {
	return NotFound(CodeSmokeCriterionNotFound, fmt.Sprintf("smoke criterion %d does not exist", id))
}

// ---------------------------------------------------------------------------
// Criteria
// ---------------------------------------------------------------------------

// Criteria returns the criteria by sort order then name; activeOnly leaves
// out archived ones.
func (s *SmokeReportService) Criteria(ctx context.Context, activeOnly bool) ([]SmokeCriterion, error) {
	return listSmokeCriteria(ctx, s.deps.DB, activeOnly)
}

func listSmokeCriteria(ctx context.Context, q querier, activeOnly bool) ([]SmokeCriterion, error) {
	query := "SELECT id, name, sort_order, active FROM smoke_report_criterion"
	if activeOnly {
		query += " WHERE active = 1"
	}
	rows, err := q.QueryContext(ctx, query+" ORDER BY sort_order, name, id")
	if err != nil {
		return nil, fmt.Errorf("list smoke criteria: %w", err)
	}
	defer rows.Close()
	out := []SmokeCriterion{}
	for rows.Next() {
		var c SmokeCriterion
		var active int
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder, &active); err != nil {
			return nil, fmt.Errorf("scan smoke criterion: %w", err)
		}
		c.Active = active == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func checkCriterionName(v *Validation, name string) {
	switch {
	case name == "":
		v.Add("name", "is required")
	case utf8.RuneCountInString(name) > MaxSmokeCriterionName:
		v.Add("name", fmt.Sprintf("must be at most %d characters", MaxSmokeCriterionName))
	}
}

// CreateCriterion adds an active criterion.
func (s *SmokeReportService) CreateCriterion(ctx context.Context, name string, sortOrder int) (SmokeCriterion, error) {
	name = strings.TrimSpace(name)
	var v Validation
	checkCriterionName(&v, name)
	if err := v.Err(); err != nil {
		return SmokeCriterion{}, err
	}
	var id int
	if err := s.deps.DB.QueryRowContext(ctx,
		"INSERT INTO smoke_report_criterion (name, sort_order, active) VALUES ($1, $2, 1) RETURNING id",
		name, sortOrder).Scan(&id); err != nil {
		return SmokeCriterion{}, fmt.Errorf("add smoke criterion: %w", err)
	}
	return s.afterCriterionWrite(ctx, id)
}

// UpdateCriterion renames, reorders, archives or restores a criterion.
// Scores keep pointing at it and take the new name.
func (s *SmokeReportService) UpdateCriterion(ctx context.Context, id int, name string, sortOrder int, active bool) (SmokeCriterion, error) {
	name = strings.TrimSpace(name)
	var v Validation
	checkCriterionName(&v, name)
	if err := v.Err(); err != nil {
		return SmokeCriterion{}, err
	}
	res, err := s.deps.DB.ExecContext(ctx,
		"UPDATE smoke_report_criterion SET name = $1, sort_order = $2, active = $3 WHERE id = $4",
		name, sortOrder, boolInt(active), id)
	if err != nil {
		return SmokeCriterion{}, fmt.Errorf("update smoke criterion %d: %w", id, err)
	}
	if err := requireRow(res, smokeCriterionNotFound(id)); err != nil {
		return SmokeCriterion{}, err
	}
	return s.afterCriterionWrite(ctx, id)
}

// DeleteCriterion deletes a criterion nobody has scored, or archives one
// that has scores; archived reports which happened.
func (s *SmokeReportService) DeleteCriterion(ctx context.Context, id int) (archived bool, err error) {
	// The check sits inside the DELETE so a score inserted concurrently
	// can't be destroyed by the cascade; a SELECT first wouldn't block it
	// under Postgres's READ COMMITTED.
	res, err := s.deps.DB.ExecContext(ctx, `
		DELETE FROM smoke_report_criterion
		WHERE id = $1 AND NOT EXISTS (SELECT 1 FROM smoke_report_score WHERE criterion_id = $1)`, id)
	if err != nil {
		return false, fmt.Errorf("delete smoke criterion %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	} else if n == 0 {
		// Scored, or missing: archiving tells them apart.
		res, err = s.deps.DB.ExecContext(ctx, "UPDATE smoke_report_criterion SET active = 0 WHERE id = $1", id)
		if err != nil {
			return false, fmt.Errorf("archive smoke criterion %d: %w", id, err)
		}
		if err := requireRow(res, smokeCriterionNotFound(id)); err != nil {
			return false, err
		}
		archived = true
	}
	return archived, refreshSmokeCriteria(ctx, s.deps.DB, s.deps.Store)
}

func (s *SmokeReportService) afterCriterionWrite(ctx context.Context, id int) (SmokeCriterion, error) {
	if err := refreshSmokeCriteria(ctx, s.deps.DB, s.deps.Store); err != nil {
		return SmokeCriterion{}, err
	}
	all, err := s.Criteria(ctx, false)
	if err != nil {
		return SmokeCriterion{}, err
	}
	for _, c := range all {
		if c.ID == id {
			return c, nil
		}
	}
	return SmokeCriterion{}, smokeCriterionNotFound(id)
}

// refreshSmokeCriteria reloads the criteria cache the plant page reads.
func refreshSmokeCriteria(ctx context.Context, q querier, store *config.Store) error {
	all, err := listSmokeCriteria(ctx, q, false)
	if err != nil {
		return err
	}
	out := make([]types.SmokeReportCriterion, len(all))
	for i, c := range all {
		out[i] = types.SmokeReportCriterion{ID: c.ID, Name: c.Name, SortOrder: c.SortOrder, Active: c.Active}
	}
	store.SetSmokeCriteria(out)
	return nil
}

// ---------------------------------------------------------------------------
// Reports
// ---------------------------------------------------------------------------

// harvestDay is the plant's first day in a harvested status (Drying,
// Curing or Success), and whether it has one.
func (s *SmokeReportService) harvestDay(ctx context.Context, plantID int) (chrono.Date, bool, error) {
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT ps.status, psl.date FROM plant_status_log psl
		JOIN plant_status ps ON ps.id = psl.status_id
		WHERE psl.plant_id = $1`, plantID)
	if err != nil {
		return chrono.Date{}, false, fmt.Errorf("plant %d status history: %w", plantID, err)
	}
	defer rows.Close()
	var first chrono.Date
	found := false
	for rows.Next() {
		var (
			name string
			at   chrono.Stored
		)
		if err := rows.Scan(&name, &at); err != nil {
			return chrono.Date{}, false, fmt.Errorf("scan status entry: %w", err)
		}
		if !StatusName(name).IsHarvested() || !at.Valid {
			continue
		}
		day := s.deps.Zones.DateOf(s.deps.Zones.FromWall(at.Time))
		if !found || day.Before(first) {
			first, found = day, true
		}
	}
	return first, found, rows.Err()
}

// Eligibility says whether a plant may get a new report (it has been
// harvested) and whether it has reports already. A plant whose harvest
// entry was deleted keeps its existing reports.
func (s *SmokeReportService) Eligibility(ctx context.Context, plantID int) (harvested, hasReports bool, err error) {
	if _, harvested, err = s.harvestDay(ctx, plantID); err != nil {
		return false, false, err
	}
	var n int
	if err := s.deps.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM smoke_report WHERE plant_id = $1", plantID).Scan(&n); err != nil {
		return false, false, fmt.Errorf("count smoke reports: %w", err)
	}
	return harvested, n > 0, nil
}

// ForPlant returns a plant's reports, latest first.
func (s *SmokeReportService) ForPlant(ctx context.Context, plantID int) ([]SmokeReport, error) {
	if err := requirePlant(ctx, s.deps.DB, plantID); err != nil {
		return nil, err
	}
	return s.query(ctx, "plant_id = $1", plantID)
}

// Get returns one report.
func (s *SmokeReportService) Get(ctx context.Context, id int) (SmokeReport, error) {
	out, err := s.query(ctx, "id = $1", id)
	if err != nil {
		return SmokeReport{}, err
	}
	if len(out) == 0 {
		return SmokeReport{}, smokeReportNotFound(id)
	}
	return out[0], nil
}

func (s *SmokeReportService) query(ctx context.Context, where string, arg int) ([]SmokeReport, error) {
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT id, plant_id, report_date, consumption_method, rating, description
		FROM smoke_report WHERE `+where, arg)
	if err != nil {
		return nil, fmt.Errorf("list smoke reports: %w", err)
	}
	defer rows.Close()
	out := []SmokeReport{}
	for rows.Next() {
		var r SmokeReport
		if err := rows.Scan(&r.ID, &r.PlantID, &r.ReportDate, &r.ConsumptionMethod, &r.Rating, &r.Description); err != nil {
			return nil, fmt.Errorf("scan smoke report: %w", err)
		}
		r.Scores = []SmokeScore{}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list smoke reports: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}
	// Latest first; stored dates come in several layouts, so sort the days.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ReportDate != out[j].ReportDate {
			return out[j].ReportDate.Before(out[i].ReportDate)
		}
		return out[i].ID > out[j].ID
	})

	byID := map[int]int{}
	ids := make([]int, len(out))
	harvests := map[int]*chrono.Date{}
	for i, r := range out {
		byID[r.ID] = i
		ids[i] = r.ID
		if _, done := harvests[r.PlantID]; !done {
			day, ok, err := s.harvestDay(ctx, r.PlantID)
			if err != nil {
				return nil, err
			}
			harvests[r.PlantID] = nil
			if ok {
				harvests[r.PlantID] = &day
			}
		}
		if h := harvests[r.PlantID]; h != nil && !r.ReportDate.Before(*h) {
			d := r.ReportDate.DaysSince(*h)
			out[i].CureDays = &d
		}
	}

	scores, err := s.deps.DB.QueryContext(ctx, fmt.Sprintf(`
		SELECT s.report_id, c.id, c.name, s.value
		FROM smoke_report_score s JOIN smoke_report_criterion c ON c.id = s.criterion_id
		WHERE s.report_id IN (%s)
		ORDER BY c.sort_order, c.name, c.id`, Placeholders(1, len(ids))), toAny(ids)...)
	if err != nil {
		return nil, fmt.Errorf("smoke report scores: %w", err)
	}
	defer scores.Close()
	for scores.Next() {
		var reportID int
		var sc SmokeScore
		if err := scores.Scan(&reportID, &sc.Criterion.ID, &sc.Criterion.Name, &sc.Value); err != nil {
			return nil, fmt.Errorf("scan smoke score: %w", err)
		}
		i := byID[reportID]
		out[i].Scores = append(out[i].Scores, sc)
	}
	return out, scores.Err()
}

// validate checks a report's content against the active criteria.
func (s *SmokeReportService) validate(ctx context.Context, in *SmokeReportInput) error {
	in.ConsumptionMethod = strings.TrimSpace(in.ConsumptionMethod)
	in.Description = strings.TrimSpace(in.Description)
	if in.ReportDate.IsZero() {
		in.ReportDate = s.deps.Zones.Today(s.deps.Clock)
	}
	var v Validation
	rangeProblem := fmt.Sprintf("must be from %d to %d", SmokeRatingMin, SmokeRatingMax)
	if in.Rating < SmokeRatingMin || in.Rating > SmokeRatingMax {
		v.Add("rating", rangeProblem)
	}
	if utf8.RuneCountInString(in.ConsumptionMethod) > MaxConsumptionMethod {
		v.Add("consumption_method", fmt.Sprintf("must be at most %d characters", MaxConsumptionMethod))
	}
	if utf8.RuneCountInString(in.Description) > utils.MaxNotesLength {
		v.Add("description", fmt.Sprintf("must be at most %d characters", utils.MaxNotesLength))
	}
	if len(in.Scores) > 0 {
		criteria, err := s.Criteria(ctx, true)
		if err != nil {
			return err
		}
		active := map[int]bool{}
		for _, c := range criteria {
			active[c.ID] = true
		}
		seen := map[int]bool{}
		for i, sc := range in.Scores {
			field := fmt.Sprintf("scores[%d]", i)
			switch {
			case !active[sc.CriterionID]:
				v.Add(field+".criterion_id", "is not an active criterion")
			case seen[sc.CriterionID]:
				v.Add(field+".criterion_id", "is scored twice")
			case sc.Value < SmokeRatingMin || sc.Value > SmokeRatingMax:
				v.Add(field+".value", rangeProblem)
			}
			seen[sc.CriterionID] = true
		}
	}
	return v.Err()
}

// writeScores replaces a report's scores on active criteria. Scores on
// archived criteria stay: the edit form can't show those criteria, so
// leaving them out of a request says nothing about them.
func writeSmokeScores(ctx context.Context, tx *sql.Tx, reportID int, scores []SmokeScoreInput) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM smoke_report_score
		WHERE report_id = $1
		  AND criterion_id IN (SELECT id FROM smoke_report_criterion WHERE active = 1)`, reportID); err != nil {
		return fmt.Errorf("clear smoke scores: %w", err)
	}
	for _, sc := range scores {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO smoke_report_score (report_id, criterion_id, value) VALUES ($1, $2, $3)",
			reportID, sc.CriterionID, sc.Value); err != nil {
			return fmt.Errorf("add smoke score: %w", err)
		}
	}
	return nil
}

// Create adds a report to a plant that has been harvested.
func (s *SmokeReportService) Create(ctx context.Context, plantID int, in SmokeReportInput) (SmokeReport, error) {
	if err := s.validate(ctx, &in); err != nil {
		return SmokeReport{}, err
	}
	if err := requirePlant(ctx, s.deps.DB, plantID); err != nil {
		return SmokeReport{}, err
	}
	harvested, _, err := s.Eligibility(ctx, plantID)
	if err != nil {
		return SmokeReport{}, err
	}
	if !harvested {
		return SmokeReport{}, Conflict(CodePlantNotHarvested, fmt.Sprintf(
			"plant %d hasn't been harvested (Drying, Curing or Success)", plantID))
	}
	var id int
	err = inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO smoke_report (plant_id, report_date, consumption_method, rating, description)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			plantID, in.ReportDate, in.ConsumptionMethod, in.Rating, in.Description).Scan(&id); err != nil {
			return fmt.Errorf("add smoke report: %w", err)
		}
		return writeSmokeScores(ctx, tx, id, in.Scores)
	})
	if err != nil {
		return SmokeReport{}, err
	}
	return s.Get(ctx, id)
}

// Update replaces a report's content and its scores on active criteria.
func (s *SmokeReportService) Update(ctx context.Context, id int, in SmokeReportInput) (SmokeReport, error) {
	if err := s.validate(ctx, &in); err != nil {
		return SmokeReport{}, err
	}
	err := inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE smoke_report
			SET report_date = $1, consumption_method = $2, rating = $3, description = $4,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $5`,
			in.ReportDate, in.ConsumptionMethod, in.Rating, in.Description, id)
		if err != nil {
			return fmt.Errorf("update smoke report %d: %w", id, err)
		}
		if err := requireRow(res, smokeReportNotFound(id)); err != nil {
			return err
		}
		return writeSmokeScores(ctx, tx, id, in.Scores)
	})
	if err != nil {
		return SmokeReport{}, err
	}
	return s.Get(ctx, id)
}

// Delete deletes a report and its scores. The scores go explicitly: SQLite
// enforces the cascade only with foreign_keys on.
func (s *SmokeReportService) Delete(ctx context.Context, id int) error {
	return inTx(ctx, s.deps.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM smoke_report_score WHERE report_id = $1", id); err != nil {
			return fmt.Errorf("delete smoke scores: %w", err)
		}
		res, err := tx.ExecContext(ctx, "DELETE FROM smoke_report WHERE id = $1", id)
		if err != nil {
			return fmt.Errorf("delete smoke report %d: %w", id, err)
		}
		return requireRow(res, smokeReportNotFound(id))
	})
}

// StrainSummary averages a strain's harvests (see SmokeSummary).
func (s *SmokeReportService) StrainSummary(ctx context.Context, strainID int) (SmokeSummary, error) {
	if _, err := (&StrainService{deps: s.deps}).Get(ctx, strainID); err != nil {
		return SmokeSummary{}, err
	}
	rows, err := s.deps.DB.QueryContext(ctx, `
		SELECT p.id, p.name, sr.id, sr.rating, sr.report_date
		FROM plant p JOIN smoke_report sr ON sr.plant_id = p.id
		WHERE p.strain_id = $1`, strainID)
	if err != nil {
		return SmokeSummary{}, fmt.Errorf("strain %d smoke reports: %w", strainID, err)
	}
	defer rows.Close()
	type row struct {
		e  SmokeSummaryEntry
		id int
	}
	latest := map[int]row{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.e.Plant.ID, &r.e.Plant.Name, &r.id, &r.e.Rating, &r.e.ReportDate); err != nil {
			return SmokeSummary{}, fmt.Errorf("scan smoke summary: %w", err)
		}
		cur, ok := latest[r.e.Plant.ID]
		if !ok || cur.e.ReportDate.Before(r.e.ReportDate) || (cur.e.ReportDate == r.e.ReportDate && r.id > cur.id) {
			latest[r.e.Plant.ID] = r
		}
	}
	if err := rows.Err(); err != nil {
		return SmokeSummary{}, fmt.Errorf("strain %d smoke reports: %w", strainID, err)
	}
	out := SmokeSummary{Entries: []SmokeSummaryEntry{}}
	total := 0
	for _, r := range latest {
		out.Entries = append(out.Entries, r.e)
		total += r.e.Rating
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.ReportDate != b.ReportDate {
			return b.ReportDate.Before(a.ReportDate)
		}
		return a.Plant.ID > b.Plant.ID
	})
	if len(out.Entries) > 0 {
		out.Average = float64(total) / float64(len(out.Entries))
	}
	return out, nil
}
