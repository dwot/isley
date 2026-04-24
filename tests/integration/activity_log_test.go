package integration

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// TestActivityLog verifies the /activities family of endpoints added for
// issue #148: the public JSON list, the per-plant and per-filter CSV/XLSX
// exports, and the auth gate on the exports.
//
// Tests in this file create their own fixtures (zone/breeder/strain/plant/
// activities) rather than depending on TestMainFlow state, which tears the
// activity down as part of its own delete tests.
func TestActivityLog(t *testing.T) {
	LoginAsAdmin(t, "newpass123")

	// --- Fixture setup ---
	zoneID := createTestZone(t, "ALog Zone")
	breederID := testAddBreeder(t, "ALog Breeder")
	strainID := testAddStrain(t, "ALog Strain", breederID)
	plantID := testAddPlant(t, "ALog Plant", strainID, zoneID)

	// Three activities: unique note so we can search for them specifically.
	const needle = "alog-needle-8231"
	activities := []struct {
		date string
		note string
	}{
		{"2024-05-01", "first " + needle + " watering"},
		{"2024-05-02", "second " + needle + " feeding"},
		{"2024-05-03", "unrelated note that doesn't match"},
	}
	for _, a := range activities {
		addPlantActivityWithNote(t, plantID, a.date, a.note)
	}

	t.Run("list returns activities for the plant", func(t *testing.T) {
		resp, err := Client.Get(BaseURL + "/activities/list?plant_id=" + strconv.Itoa(plantID))
		if err != nil {
			t.Fatalf("GET /activities/list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 from /activities/list, got %d", resp.StatusCode)
		}
		var payload struct {
			Entries    []map[string]any `json:"entries"`
			Total      int              `json:"total"`
			Page       int              `json:"page"`
			TotalPages int              `json:"total_pages"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode /activities/list: %v", err)
		}
		if payload.Total != len(activities) {
			t.Errorf("expected total=%d, got %d", len(activities), payload.Total)
		}
		if len(payload.Entries) != len(activities) {
			t.Errorf("expected %d entries, got %d", len(activities), len(payload.Entries))
		}
	})

	t.Run("list q= filters by note text", func(t *testing.T) {
		resp, err := Client.Get(BaseURL + "/activities/list?plant_id=" + strconv.Itoa(plantID) + "&q=" + needle)
		if err != nil {
			t.Fatalf("GET /activities/list?q: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
		var payload struct {
			Total int `json:"total"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload.Total != 2 {
			t.Errorf("expected 2 entries matching needle, got %d", payload.Total)
		}
	})

	t.Run("list from/to filters by date range", func(t *testing.T) {
		q := url.Values{}
		q.Set("plant_id", strconv.Itoa(plantID))
		q.Set("from", "2024-05-02")
		q.Set("to", "2024-05-02")
		resp, err := Client.Get(BaseURL + "/activities/list?" + q.Encode())
		if err != nil {
			t.Fatalf("GET /activities/list?date-range: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
		var payload struct {
			Total   int              `json:"total"`
			Entries []map[string]any `json:"entries"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if payload.Total != 1 {
			t.Errorf("expected 1 entry on 2024-05-02, got %d", payload.Total)
		}
		if len(payload.Entries) != 1 {
			t.Errorf("expected 1 entry in body, got %d", len(payload.Entries))
		}
	})

	t.Run("csv export returns correct content-type and rows", func(t *testing.T) {
		resp, err := Client.Get(BaseURL + "/activities/export/csv?plant_id=" + strconv.Itoa(plantID))
		if err != nil {
			t.Fatalf("GET /activities/export/csv: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 from CSV export, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
			t.Errorf("expected text/csv Content-Type, got %q", ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".csv") {
			t.Errorf("expected attachment .csv Content-Disposition, got %q", cd)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rows, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
		if err != nil {
			t.Fatalf("parse CSV: %v", err)
		}
		if len(rows) != len(activities)+1 { // +1 for header
			t.Errorf("expected %d CSV rows (header + %d entries), got %d", len(activities)+1, len(activities), len(rows))
		}
		// Header sanity check
		expectedHeader := []string{"Date", "Plant", "Strain", "Zone", "Activity", "Note"}
		if len(rows) > 0 {
			for i, h := range expectedHeader {
				if i >= len(rows[0]) || rows[0][i] != h {
					t.Errorf("unexpected CSV header row: %v", rows[0])
					break
				}
			}
		}
	})

	t.Run("xlsx export returns correct content-type and valid zip", func(t *testing.T) {
		resp, err := Client.Get(BaseURL + "/activities/export/xlsx?plant_id=" + strconv.Itoa(plantID))
		if err != nil {
			t.Fatalf("GET /activities/export/xlsx: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 from XLSX export, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			t.Errorf("expected xlsx Content-Type, got %q", ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".xlsx") {
			t.Errorf("expected attachment .xlsx Content-Disposition, got %q", cd)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		// xlsx files are zip archives.  Verify the magic bytes and that the
		// zip parses.  Also confirm at least the core xl/workbook.xml entry
		// is present.
		if len(body) < 4 || string(body[0:2]) != "PK" {
			head := 4
			if len(body) < head {
				head = len(body)
			}
			t.Fatalf("response does not start with PK zip magic; got first bytes %v", body[:head])
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			t.Fatalf("xlsx body is not a valid zip: %v", err)
		}
		hasWorkbook := false
		for _, f := range zr.File {
			if f.Name == "xl/workbook.xml" {
				hasWorkbook = true
				break
			}
		}
		if !hasWorkbook {
			t.Errorf("xlsx archive missing xl/workbook.xml entry")
		}
	})

	t.Run("exports require login", func(t *testing.T) {
		// Log out, then hit the export endpoints — we expect a redirect to /login.
		logoutResp, err := Client.Get(BaseURL + "/logout")
		if err != nil {
			t.Fatalf("logout: %v", err)
		}
		logoutResp.Body.Close()

		for _, path := range []string{"/activities/export/csv", "/activities/export/xlsx"} {
			resp, err := Client.Get(BaseURL + path + "?plant_id=" + strconv.Itoa(plantID))
			if err != nil {
				t.Fatalf("GET %s (logged out): %v", path, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusFound {
				t.Errorf("expected 302 from %s when logged out, got %d", path, resp.StatusCode)
				continue
			}
			if loc := resp.Header.Get("Location"); loc != "/login" {
				t.Errorf("expected Location=/login from %s, got %q", path, loc)
			}
		}

		// Restore login for any subsequent tests that share this client/cookie jar.
		LoginAsAdmin(t, "newpass123")
	})
}

// addPlantActivityWithNote is a thin variant of the existing addPlantActivity
// helper that lets the caller specify the date and note — needed because the
// test asserts on date ranges and note text.
func addPlantActivityWithNote(t *testing.T, plantID int, date, note string) int {
	t.Helper()
	LoginAsAdmin(t, "newpass123")
	payload := map[string]any{
		"plant_id":    plantID,
		"activity_id": 1, // "Water" seed activity, always present
		"date":        date,
		"note":        note,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/plantActivity", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("build plant activity POST: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("send plant activity POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 200 or 201 from /plantActivity, got %d", resp.StatusCode)
	}
	// The handler returns the original input payload (plant_id field); that's
	// fine for our purposes — we don't use the returned ID directly.
	return plantID
}
