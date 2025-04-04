package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSettingsFlow(t *testing.T) {

	t.Run("test add stream", testAddStream)
	t.Run("test stream appears in settings", testStreamAppearsInSettings)
}

func createTestZone(t *testing.T, zoneName string) string {
	payload := map[string]any{
		"zone_name": zoneName,
	}
	data, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", BaseURL+"/zones", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build zone POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send zone POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /zones, got %d", resp.StatusCode)
	}

	var result struct {
		ID string `json:"id"` // or whatever your handler returns
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /zones: %v", err)
	}

	if result.ID == "" {
		t.Fatalf("Zone ID missing from response")
	}

	t.Logf("Created test zone: %s", result.ID)
	return result.ID
}

func testAddStream(t *testing.T) {
	// Ensure we're logged in with an admin session
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test

	payload := map[string]any{
		"stream_name": "Test Stream 1",
		"zone_id":     "zone-abc",
		"url":         "http://example.com/stream.m3u8",
		"visible":     true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	req, err := http.NewRequest("POST", BaseURL+"/streams", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201, got %d", resp.StatusCode)
	}

	// Optionally decode response JSON:
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response JSON: %v", err)
	}

	t.Logf("Stream added successfully: %+v", result)
}

func testStreamAppearsInSettings(t *testing.T) {
	resp, err := Client.Get(BaseURL + "/settings")
	if err != nil {
		t.Fatalf("Failed to GET /settings: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /settings, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	html := string(body)

	// Look for a known unique piece of the stream we added
	if !strings.Contains(html, "Test Stream 1") {
		t.Errorf("Expected stream name 'Test Stream 1' to appear in /settings page HTML")
	}
	if !strings.Contains(html, "http://example.com/stream.m3u8") {
		t.Errorf("Expected stream URL to appear in /settings page HTML")
	}
}
