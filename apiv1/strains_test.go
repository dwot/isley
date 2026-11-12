package apiv1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"isley/tests/testutil"
)

type breederBody struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	StrainCount int    `json:"strain_count"`
}

type strainBody struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Breeder struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"breeder"`
	IndicaPct     int    `json:"indica_pct"`
	SativaPct     int    `json:"sativa_pct"`
	Autoflower    bool   `json:"autoflower"`
	Feminized     bool   `json:"feminized"`
	SeedCount     int    `json:"seed_count"`
	CycleTimeDays *int   `json:"cycle_time_days"`
	URL           string `json:"url"`
	Parents       []struct {
		Name   string `json:"name"`
		Strain *struct {
			ID int `json:"id"`
		} `json:"strain"`
	} `json:"parents"`
}

type ancestorBody struct {
	Name    string         `json:"name"`
	Parents []ancestorBody `json:"parents"`
}

func TestBreedersAPI(t *testing.T) {
	t.Parallel()
	checkBreedersAPI(t, newFixture(t))
}

func checkBreedersAPI(t *testing.T, f fixture) {
	read, write, admin := f.key(t, "read"), f.key(t, "write"), f.key(t, "admin")

	resp := f.cc.do(http.MethodPost, "/breeders", bearer(write), []byte(`{"name":" Acme "}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	acme := decode[breederBody](t, resp.Body)
	assert.Equal(t, "Acme", acme.Name)
	assert.Equal(t, "/api/v1/breeders/"+itoa(acme.ID), resp.Header.Get("Location"))

	resp = f.cc.do(http.MethodPost, "/breeders", bearer(write), []byte(`{"name":"ACME"}`))
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "breeder_name_taken", requireErrorEnvelope(t, resp.Body).Error.Code)

	resp = f.cc.do(http.MethodPost, "/breeders", bearer(write), []byte(`{"name":"Zeta"}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	zeta := decode[breederBody](t, resp.Body)
	resp = f.cc.do(http.MethodPatch, "/breeders/"+itoa(zeta.ID), bearer(write), []byte(`{"name":"Beta"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, "Beta", decode[breederBody](t, resp.Body).Name)

	testutil.SeedStrain(t, f.server.DB, acme.ID, "S")
	resp = f.cc.get("/breeders?limit=1", read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	page := decode[struct {
		Items      []breederBody `json:"items"`
		NextCursor *string       `json:"next_cursor"`
	}](t, resp.Body)
	require.Len(t, page.Items, 1)
	assert.Equal(t, breederBody{ID: acme.ID, Name: "Acme", StrainCount: 1}, page.Items[0])
	require.NotNil(t, page.NextCursor)

	assert.Equal(t, http.StatusForbidden, f.cc.do(http.MethodDelete, "/breeders/"+itoa(zeta.ID), bearer(write), nil).StatusCode)
	resp = f.cc.do(http.MethodDelete, "/breeders/"+itoa(acme.ID), bearer(admin), nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "breeder_not_empty", requireErrorEnvelope(t, resp.Body).Error.Code)
	assert.Equal(t, http.StatusNoContent, f.cc.do(http.MethodDelete, "/breeders/"+itoa(zeta.ID), bearer(admin), nil).StatusCode)
	assert.Equal(t, http.StatusNotFound, f.cc.get("/breeders/"+itoa(zeta.ID), read).StatusCode)
}

func TestStrainsAPI(t *testing.T) {
	t.Parallel()
	checkStrainsAPI(t, newFixture(t))
}

func checkStrainsAPI(t *testing.T, f fixture) {
	db := f.server.DB
	read, write, admin := f.key(t, "read"), f.key(t, "write"), f.key(t, "admin")

	// A breeder by name is made on the spot; one percentage sets the other.
	resp := f.cc.do(http.MethodPost, "/strains", bearer(write),
		[]byte(`{"name":"Blue Dream","breeder_name":"Humboldt","sativa_pct":70,"seed_count":3,"cycle_time_days":63,"url":"www.example.com/bd"}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	bd := decode[strainBody](t, resp.Body)
	assert.Equal(t, "/api/v1/strains/"+itoa(bd.ID), resp.Header.Get("Location"))
	assert.Equal(t, "Humboldt", bd.Breeder.Name)
	assert.Equal(t, 30, bd.IndicaPct)
	assert.Equal(t, 70, bd.SativaPct)
	assert.Equal(t, "https://www.example.com/bd", bd.URL)
	require.NotNil(t, bd.CycleTimeDays)
	assert.Empty(t, bd.Parents)

	resp = f.cc.do(http.MethodPost, "/strains", bearer(write),
		[]byte(`{"name":"OG","breeder_id":`+itoa(bd.Breeder.ID)+`,"autoflower":true}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	og := decode[strainBody](t, resp.Body)
	assert.Equal(t, 50, og.IndicaPct)
	assert.Nil(t, og.CycleTimeDays)

	resp = f.cc.do(http.MethodPost, "/strains", bearer(write), []byte(`{"name":"x","indica_pct":60,"sativa_pct":60}`))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	fields := requireErrorEnvelope(t, resp.Body).Error.Details["fields"].(map[string]any)
	assert.Contains(t, fields, "breeder_id")
	assert.Contains(t, fields, "sativa_pct")

	// PATCH: one percentage moves the other.
	resp = f.cc.do(http.MethodPatch, "/strains/"+itoa(og.ID), bearer(write), []byte(`{"indica_pct":80,"cycle_time_days":null}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	og = decode[strainBody](t, resp.Body)
	assert.Equal(t, 20, og.SativaPct)
	assert.True(t, og.Autoflower)

	// Seeds go by delta and never below zero.
	resp = f.cc.do(http.MethodPost, "/strains/"+itoa(bd.ID)+"/seed-count", bearer(write), []byte(`{"delta":-2}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.Equal(t, 1, decode[strainBody](t, resp.Body).SeedCount)
	resp = f.cc.do(http.MethodPost, "/strains/"+itoa(bd.ID)+"/seed-count", bearer(write), []byte(`{"delta":-2}`))
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "not_enough_seeds", requireErrorEnvelope(t, resp.Body).Error.Code)
	assert.Equal(t, http.StatusBadRequest,
		f.cc.do(http.MethodPost, "/strains/"+itoa(bd.ID)+"/seed-count", bearer(write), []byte(`{}`)).StatusCode)

	// Lineage: OG is a parent of a cross; the cross can't then parent OG.
	resp = f.cc.do(http.MethodPost, "/strains", bearer(write),
		[]byte(`{"name":"Cross","breeder_id":`+itoa(bd.Breeder.ID)+`}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	cross := decode[strainBody](t, resp.Body)
	resp = f.cc.do(http.MethodPut, "/strains/"+itoa(cross.ID)+"/lineage", bearer(write),
		[]byte(`{"parents":[{"strain_id":`+itoa(og.ID)+`},{"name":"Haze"}]}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.do(http.MethodPut, "/strains/"+itoa(og.ID)+"/lineage", bearer(write),
		[]byte(`{"parents":[{"name":"Landrace"}]}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	resp = f.cc.get("/strains/"+itoa(cross.ID)+"/lineage", read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	tree := decode[struct {
		Parents []ancestorBody `json:"parents"`
	}](t, resp.Body)
	require.Len(t, tree.Parents, 2)
	assert.Equal(t, "Haze", tree.Parents[0].Name)
	assert.Equal(t, "OG", tree.Parents[1].Name)
	require.Len(t, tree.Parents[1].Parents, 1)
	assert.Equal(t, "Landrace", tree.Parents[1].Parents[0].Name)

	resp = f.cc.do(http.MethodPut, "/strains/"+itoa(og.ID)+"/lineage", bearer(write),
		[]byte(`{"parents":[{"strain_id":`+itoa(cross.ID)+`}]}`))
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "lineage_cycle", requireErrorEnvelope(t, resp.Body).Error.Code)

	resp = f.cc.get("/strains/"+itoa(og.ID)+"/descendants", read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	desc := decode[struct {
		Items []struct {
			Name         string   `json:"name"`
			OtherParents []string `json:"other_parents"`
		} `json:"items"`
	}](t, resp.Body)
	require.Len(t, desc.Items, 1)
	assert.Equal(t, "Cross", desc.Items[0].Name)
	assert.Equal(t, []string{"Haze"}, desc.Items[0].OtherParents)

	// Lists filter and page by name; each strain carries its parents.
	resp = f.cc.get("/strains?in_stock=true", read)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	list := decode[struct {
		Items      []strainBody `json:"items"`
		NextCursor *string      `json:"next_cursor"`
	}](t, resp.Body)
	require.Len(t, list.Items, 1)
	assert.Equal(t, "Blue Dream", list.Items[0].Name)
	resp = f.cc.get("/strains?q=cRoSs", read)
	list = decode[struct {
		Items      []strainBody `json:"items"`
		NextCursor *string      `json:"next_cursor"`
	}](t, resp.Body)
	require.Len(t, list.Items, 1)
	require.Len(t, list.Items[0].Parents, 2)
	require.NotNil(t, list.Items[0].Parents[1].Strain)
	assert.Equal(t, og.ID, list.Items[0].Parents[1].Strain.ID)
	assert.Equal(t, http.StatusBadRequest, f.cc.get("/strains?in_stock=maybe", read).StatusCode)

	// A strain with plants can't be deleted; one without can, by admin.
	testutil.SeedPlant(t, db, "P", bd.ID, testutil.SeedZone(t, db, "Tent"))
	resp = f.cc.do(http.MethodDelete, "/strains/"+itoa(bd.ID), bearer(admin), nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "strain_in_use", requireErrorEnvelope(t, resp.Body).Error.Code)
	assert.Equal(t, http.StatusForbidden, f.cc.do(http.MethodDelete, "/strains/"+itoa(og.ID), bearer(write), nil).StatusCode)
	assert.Equal(t, http.StatusNoContent, f.cc.do(http.MethodDelete, "/strains/"+itoa(og.ID), bearer(admin), nil).StatusCode)
	resp = f.cc.get("/strains/"+itoa(cross.ID), read)
	cross = decode[strainBody](t, resp.Body)
	require.Len(t, cross.Parents, 2)
	assert.Equal(t, "OG", cross.Parents[1].Name, "the name stays")
	assert.Nil(t, cross.Parents[1].Strain, "the link goes")
	assert.Equal(t, http.StatusNotFound, f.cc.get("/strains/"+itoa(og.ID), read).StatusCode)
}

func TestStrainsAPI_Feminized(t *testing.T) {
	t.Parallel()
	checkStrainsAPIFeminized(t, newFixture(t))
}

// checkStrainsAPIFeminized: strains carry feminized (default true), it can be
// set on create and patched, and the list filters on it.
func checkStrainsAPIFeminized(t *testing.T, f fixture) {
	read, write := f.key(t, "read"), f.key(t, "write")

	resp := f.cc.do(http.MethodPost, "/strains", bearer(write), []byte(`{"name":"Fem","breeder_name":"Acme"}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	fem := decode[strainBody](t, resp.Body)
	assert.True(t, fem.Feminized, "defaults to feminized")

	resp = f.cc.do(http.MethodPost, "/strains", bearer(write),
		[]byte(`{"name":"Reg","breeder_id":`+itoa(fem.Breeder.ID)+`,"feminized":false}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "%s", resp.Body)
	reg := decode[strainBody](t, resp.Body)
	assert.False(t, reg.Feminized)

	list := func(q string) []string {
		resp := f.cc.do(http.MethodGet, "/strains"+q, bearer(read), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
		page := decode[struct {
			Items []strainBody `json:"items"`
		}](t, resp.Body)
		names := []string{}
		for _, s := range page.Items {
			names = append(names, s.Name)
		}
		return names
	}
	assert.Equal(t, []string{"Reg"}, list("?feminized=false"))
	assert.Equal(t, []string{"Fem"}, list("?feminized=true"))
	resp = f.cc.do(http.MethodGet, "/strains?feminized=maybe", bearer(read), nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Contains(t, requireErrorEnvelope(t, resp.Body).Error.Details["fields"].(map[string]any), "feminized")

	resp = f.cc.do(http.MethodPatch, "/strains/"+itoa(fem.ID), bearer(write), []byte(`{"feminized":false}`))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", resp.Body)
	assert.False(t, decode[strainBody](t, resp.Body).Feminized)
	resp = f.cc.do(http.MethodGet, "/strains/"+itoa(fem.ID), bearer(read), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.False(t, decode[strainBody](t, resp.Body).Feminized)
}
