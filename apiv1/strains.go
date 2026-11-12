package apiv1

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"isley/service"
)

// ---------------------------------------------------------------------------
// Breeders
// ---------------------------------------------------------------------------

type breederJSON struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	StrainCount int    `json:"strain_count"`
}

func toBreederJSON(b service.Breeder) breederJSON {
	return breederJSON{ID: b.ID, Name: b.Name, StrainCount: b.StrainCount}
}

func (a *api) listBreeders(c *gin.Context) {
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	bs, total, err := a.svc.Breeders.List(c.Request.Context(), offset, page.Limit)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]breederJSON, len(bs))
	for i, b := range bs {
		out[i] = toBreederJSON(b)
	}
	next, ok := nextOffset(c, offset, len(out), total)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

func (a *api) getBreeder(c *gin.Context) {
	id, ok := pathID(c, service.CodeBreederNotFound, "breeder")
	if !ok {
		return
	}
	b, err := a.svc.Breeders.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toBreederJSON(b))
}

func (a *api) createBreeder(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if !bindJSON(c, &req) {
		return
	}
	b, err := a.svc.Breeders.Create(c.Request.Context(), req.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Location", Prefix+"/breeders/"+itoa(b.ID))
	c.JSON(http.StatusCreated, toBreederJSON(b))
}

func (a *api) updateBreeder(c *gin.Context) {
	id, ok := pathID(c, service.CodeBreederNotFound, "breeder")
	if !ok {
		return
	}
	var req struct {
		Name optional[string] `json:"name"`
	}
	if !bindJSON(c, &req) {
		return
	}
	ctx := c.Request.Context()
	var (
		b   service.Breeder
		err error
	)
	if req.Name.set {
		name := ""
		if req.Name.value != nil {
			name = *req.Name.value
		}
		b, err = a.svc.Breeders.Rename(ctx, id, name)
	} else {
		b, err = a.svc.Breeders.Get(ctx, id)
	}
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toBreederJSON(b))
}

func (a *api) deleteBreeder(c *gin.Context) {
	id, ok := pathID(c, service.CodeBreederNotFound, "breeder")
	if !ok {
		return
	}
	if err := a.svc.Breeders.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Strains
// ---------------------------------------------------------------------------

type lineageEntryJSON struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Strain *refJSON `json:"strain"`
}

type strainJSON struct {
	ID               int                `json:"id"`
	Name             string             `json:"name"`
	Breeder          refJSON            `json:"breeder"`
	IndicaPct        int                `json:"indica_pct"`
	SativaPct        int                `json:"sativa_pct"`
	Autoflower       bool               `json:"autoflower"`
	Feminized        bool               `json:"feminized"`
	SeedCount        int                `json:"seed_count"`
	CycleTimeDays    *int               `json:"cycle_time_days"`
	URL              string             `json:"url"`
	ShortDescription string             `json:"short_description"`
	Description      string             `json:"description"`
	Parents          []lineageEntryJSON `json:"parents"`
}

func toLineageEntryJSON(e service.LineageEntry) lineageEntryJSON {
	out := lineageEntryJSON{ID: e.ID, Name: e.ParentName}
	if e.Parent != nil {
		out.Strain = &refJSON{ID: e.Parent.ID, Name: e.Parent.Name}
	}
	return out
}

func toStrainJSON(st service.Strain) strainJSON {
	out := strainJSON{
		ID: st.ID, Name: st.Name, Breeder: refJSON{ID: st.Breeder.ID, Name: st.Breeder.Name},
		IndicaPct: st.IndicaPct, SativaPct: st.SativaPct, Autoflower: st.Autoflower, Feminized: st.Feminized,
		SeedCount: st.SeedCount, CycleTimeDays: st.CycleTimeDays, URL: st.URL,
		ShortDescription: st.ShortDescription, Description: st.Description,
		Parents: make([]lineageEntryJSON, len(st.Parents)),
	}
	for i, e := range st.Parents {
		out.Parents[i] = toLineageEntryJSON(e)
	}
	return out
}

func (a *api) listStrains(c *gin.Context) {
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	problems := map[string]string{}
	f := service.StrainFilter{
		BreederID: queryID(c, "breeder_id", problems),
		Query:     c.Query("q"),
		Offset:    offset,
		Limit:     page.Limit,
	}
	f.InStock = queryBool(c, "in_stock", problems)
	f.Feminized = queryBool(c, "feminized", problems)
	if len(problems) > 0 {
		invalid(c, problems)
		return
	}
	strains, total, err := a.svc.Strains.List(c.Request.Context(), f)
	if err != nil {
		writeError(c, err)
		return
	}
	out := make([]strainJSON, len(strains))
	for i, st := range strains {
		out[i] = toStrainJSON(st)
	}
	next, ok := nextOffset(c, offset, len(out), total)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

func (a *api) getStrain(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	st, err := a.svc.Strains.Get(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toStrainJSON(st))
}

func (a *api) createStrain(c *gin.Context) {
	var req struct {
		Name             string `json:"name"`
		BreederID        int    `json:"breeder_id"`
		BreederName      string `json:"breeder_name"`
		IndicaPct        *int   `json:"indica_pct"`
		SativaPct        *int   `json:"sativa_pct"`
		Autoflower       bool   `json:"autoflower"`
		Feminized        *bool  `json:"feminized"`
		SeedCount        int    `json:"seed_count"`
		CycleTimeDays    *int   `json:"cycle_time_days"`
		URL              string `json:"url"`
		ShortDescription string `json:"short_description"`
		Description      string `json:"description"`
	}
	if !bindJSON(c, &req) {
		return
	}
	in := service.StrainInput{
		Name: req.Name, BreederID: req.BreederID, BreederName: req.BreederName,
		IndicaPct: 50, SativaPct: 50, Autoflower: req.Autoflower, Feminized: req.Feminized, SeedCount: req.SeedCount,
		CycleTimeDays: req.CycleTimeDays, URL: req.URL,
		ShortDescription: req.ShortDescription, Description: req.Description,
	}
	// Give one percentage and the other follows.
	switch {
	case req.IndicaPct != nil && req.SativaPct != nil:
		in.IndicaPct, in.SativaPct = *req.IndicaPct, *req.SativaPct
	case req.IndicaPct != nil:
		in.IndicaPct, in.SativaPct = *req.IndicaPct, 100-*req.IndicaPct
	case req.SativaPct != nil:
		in.IndicaPct, in.SativaPct = 100-*req.SativaPct, *req.SativaPct
	}
	st, err := a.svc.Strains.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Location", Prefix+"/strains/"+itoa(st.ID))
	c.JSON(http.StatusCreated, toStrainJSON(st))
}

func (a *api) updateStrain(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	var req struct {
		Name             optional[string] `json:"name"`
		BreederID        optional[int]    `json:"breeder_id"`
		BreederName      string           `json:"breeder_name"`
		IndicaPct        optional[int]    `json:"indica_pct"`
		SativaPct        optional[int]    `json:"sativa_pct"`
		Autoflower       optional[bool]   `json:"autoflower"`
		Feminized        optional[bool]   `json:"feminized"`
		SeedCount        optional[int]    `json:"seed_count"`
		CycleTimeDays    optional[int]    `json:"cycle_time_days"`
		URL              optional[string] `json:"url"`
		ShortDescription optional[string] `json:"short_description"`
		Description      optional[string] `json:"description"`
	}
	if !bindJSON(c, &req) {
		return
	}
	upd := service.StrainUpdate{
		Name: req.Name.patch(), BreederID: req.BreederID.patch(), BreederName: req.BreederName,
		IndicaPct: req.IndicaPct.patch(), SativaPct: req.SativaPct.patch(), Autoflower: req.Autoflower.patch(),
		Feminized: req.Feminized.patch(), SeedCount: req.SeedCount.patch(), CycleTimeDays: req.CycleTimeDays.patch(), URL: req.URL.patch(),
		ShortDescription: req.ShortDescription.patch(), Description: req.Description.patch(),
	}
	// One percentage alone moves the other with it.
	if req.IndicaPct.set && !req.SativaPct.set && req.IndicaPct.value != nil {
		s := 100 - *req.IndicaPct.value
		upd.SativaPct = service.Put(&s)
	}
	if req.SativaPct.set && !req.IndicaPct.set && req.SativaPct.value != nil {
		i := 100 - *req.SativaPct.value
		upd.IndicaPct = service.Put(&i)
	}
	st, err := a.svc.Strains.Update(c.Request.Context(), id, upd)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toStrainJSON(st))
}

func (a *api) deleteStrain(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	if err := a.svc.Strains.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (a *api) adjustSeedCount(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	var req struct {
		Delta *int `json:"delta"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Delta == nil {
		invalid(c, map[string]string{"delta": "is required"})
		return
	}
	st, err := a.svc.Strains.AdjustSeeds(c.Request.Context(), id, *req.Delta)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toStrainJSON(st))
}

type ancestorJSON struct {
	lineageEntryJSON
	Parents []ancestorJSON `json:"parents"`
}

func toAncestryJSON(nodes []service.AncestryNode) []ancestorJSON {
	out := make([]ancestorJSON, len(nodes))
	for i, n := range nodes {
		out[i] = ancestorJSON{lineageEntryJSON: toLineageEntryJSON(n.LineageEntry), Parents: toAncestryJSON(n.Parents)}
	}
	return out
}

func (a *api) writeLineage(c *gin.Context, id int) {
	nodes, err := a.svc.Strains.Ancestry(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"parents": toAncestryJSON(nodes)})
}

func (a *api) getLineage(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	a.writeLineage(c, id)
}

func (a *api) setLineage(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	var req struct {
		Parents []struct {
			Name     string `json:"name"`
			StrainID *int   `json:"strain_id"`
		} `json:"parents"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Parents == nil {
		invalid(c, map[string]string{"parents": "is required (an empty list clears the lineage)"})
		return
	}
	parents := make([]service.ParentInput, len(req.Parents))
	for i, p := range req.Parents {
		parents[i] = service.ParentInput{Name: p.Name, StrainID: p.StrainID}
	}
	if _, err := a.svc.Strains.SetLineage(c.Request.Context(), id, parents); err != nil {
		writeError(c, err)
		return
	}
	a.writeLineage(c, id)
}

type descendantJSON struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Breeder      refJSON  `json:"breeder"`
	OtherParents []string `json:"other_parents"`
}

func (a *api) listDescendants(c *gin.Context) {
	id, ok := pathID(c, service.CodeStrainNotFound, "strain")
	if !ok {
		return
	}
	page, offset, ok := readOffset(c)
	if !ok {
		return
	}
	ds, err := a.svc.Strains.Descendants(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	end := min(offset+page.Limit, len(ds))
	start := min(offset, end)
	out := make([]descendantJSON, 0, end-start)
	for _, d := range ds[start:end] {
		out = append(out, descendantJSON{
			ID: d.ID, Name: d.Name, Breeder: refJSON{ID: d.Breeder.ID, Name: d.Breeder.Name}, OtherParents: d.OtherParents,
		})
	}
	next, ok := nextOffset(c, offset, len(out), len(ds))
	if !ok {
		return
	}
	c.JSON(http.StatusOK, NewList(out, next))
}

// queryBool reads an optional true/false query parameter: nil when absent.
func queryBool(c *gin.Context, name string, problems map[string]string) *bool {
	switch c.Query(name) {
	case "":
		return nil
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	default:
		problems[name] = "must be true or false"
		return nil
	}
}
