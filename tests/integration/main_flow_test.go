package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMainFlow(t *testing.T) {

	t.Run("test denied protected route", testDeniedProtectedRoute)
	t.Run("login and redirect to change-password", testInitLoginFlow)
	t.Run("access protected route after password change", testAccessProtectedRoute)
	t.Run("logout", testLogout)
	t.Run("access denied route after logout", testDeniedProtectedRoute)
	t.Run("re-login with new password", testLoginWithNewPassword)
	t.Run("access protected route after re-login", testAccessProtectedRoute)

	/*
		Additional Tests to be Designed and Implemented:
		Login & Perms
			1. Test invalid login credentials

		Settings Page
			1. Test Guest Mode
			2. Test AC Infinity Sensor Setup
			3. Test EcoWitt Sensor Setup

		Strains Page
			Add Strain
			Edit Strain
			Delete Strain

		Sensors Page
		Plants Page
		Plant Page
	*/
	zoneID := 0
	activityID := 0
	metricID := 0
	breederID := 0
	strainID := 0
	plantID := 0
	measurementID := 0
	plantActivityID := 0
	streamID := 0
	plantImageID := 0
	imagePath := ""
	t.Run("test create zone", func(t *testing.T) {
		zoneID = createTestZone(t, "Test Zone 1")
		if zoneID == 0 {
			t.Fatal("Failed to create test zone")
		}
	})
	t.Run("test add stream", func(t *testing.T) {
		streamID = testAddStream(t, zoneID)
		if streamID == 0 {
			t.Fatal("Failed to create test stream")
		}
	})
	t.Run("test stream appears in settings", testStreamAppearsInSettings)
	t.Run("test edit zone", func(t *testing.T) {
		editTestZone(t, zoneID, "Updated Zone Name")
	})
	t.Run("test delete zone", func(t *testing.T) {
		deleteTestZone(t, zoneID)
	})
	zoneID = 0
	t.Run("test add zone 2", func(t *testing.T) {
		zoneID = createTestZone(t, "Test Zone 2")
		if zoneID == 0 {
			t.Fatal("Failed to create test zone")
		}
	})
	t.Run("test edit stream", func(t *testing.T) {
		editTestStream(t, streamID, "Updated Stream Name", zoneID)
	})
	t.Run("test delete stream", func(t *testing.T) {
		deleteTestStream(t, streamID)
	})

	t.Run("test add activity", func(t *testing.T) {
		activityID = testAddActivity(t, "Test Activity")
		if activityID == 0 {
			t.Fatal("Failed to create test activity")
		}
	})
	t.Run("test activity appears in settings", func(t *testing.T) {
		testActivityAppearsInSettings(t, activityID)
	})
	t.Run("test edit activity", func(t *testing.T) {
		editTestActivity(t, activityID, "Updated Activity Name")
	})
	t.Run("test delete activity", func(t *testing.T) {
		deleteTestActivity(t, activityID)
	})
	t.Run("test add metric", func(t *testing.T) {
		metricID = testAddMetric(t, "Test Metric")
		if metricID == 0 {
			t.Fatal("Failed to create test metric")
		}
	})
	t.Run("test metric appears in settings", func(t *testing.T) {
		testMetricAppearsInSettings(t, metricID)
	})
	t.Run("test edit metric", func(t *testing.T) {
		editTestMetric(t, metricID, "Updated Metric Name")
	})
	t.Run("test delete metric", func(t *testing.T) {
		deleteTestMetric(t, metricID)
	})
	t.Run("test add breeder", func(t *testing.T) {
		breederID = testAddBreeder(t, "Test Breeder")
		if breederID == 0 {
			t.Fatal("Failed to create test breeder")
		}
	})
	t.Run("test breeder appears in settings", func(t *testing.T) {
		testBreederAppearsInSettings(t, breederID)
	})
	t.Run("test edit breeder", func(t *testing.T) {
		editTestBreeder(t, breederID, "Updated Breeder Name")
	})
	t.Run("test delete breeder", func(t *testing.T) {
		deleteTestBreeder(t, breederID)
	})
	breederID = 0
	t.Run("test add breeder", func(t *testing.T) {
		breederID = testAddBreeder(t, "Test Breeder")
		if breederID == 0 {
			t.Fatal("Failed to create test breeder")
		}
	})
	t.Run("test add strain", func(t *testing.T) {
		strainID = testAddStrain(t, "Test Strain", breederID)
		if strainID == 0 {
			t.Fatal("Failed to create test strain")
		}
	})
	t.Run("test edit strain", func(t *testing.T) {
		editTestStrain(t, strainID, breederID, "Updated Strain Name")
	})
	t.Run("test delete strain", func(t *testing.T) {
		deleteTestStrain(t, strainID)
	})
	strainID = 0
	t.Run("test add strain", func(t *testing.T) {
		strainID = testAddStrain(t, "Test Strain", breederID)
		if strainID == 0 {
			t.Fatal("Failed to create test strain")
		}
	})
	t.Run("test add plant", func(t *testing.T) {
		plantID = testAddPlant(t, "Test Plant", strainID, zoneID)
		if plantID == 0 {
			t.Fatal("Failed to create test plant")
		}
	})
	t.Run("test edit plant", func(t *testing.T) {
		editTestPlant(t, plantID, "Updated Plant Name", strainID, zoneID)
	})
	t.Run("test delete plant", func(t *testing.T) {
		deleteTestPlant(t, plantID)
	})
	plantID = 0
	t.Run("test add plant", func(t *testing.T) {
		plantID = testAddPlant(t, "Test Plant", strainID, zoneID)
		if plantID == 0 {
			t.Fatal("Failed to create test plant")
		}
	})
	t.Run("test add plant activity", func(t *testing.T) {
		plantActivityID = addPlantActivity(t, plantID)
		if plantActivityID == 0 {
			t.Fatal("Failed to create test plant activity")
		}
	})
	t.Run("test edit plant activity", func(t *testing.T) {
		editTestPlantActivity(t, plantActivityID)
	})
	t.Run("test delete plant activity", func(t *testing.T) {
		deleteTestPlantActivity(t, plantActivityID)
	})
	t.Run("test add measurement", func(t *testing.T) {
		measurementID = testAddMeasurement(t, plantID)
		if measurementID == 0 {
			t.Fatal("Failed to create test measurement")
		}
	})
	t.Run("test edit measurement", func(t *testing.T) {
		editTestMeasurement(t, measurementID)
	})
	t.Run("test delete measurement", func(t *testing.T) {
		deleteTestMeasurement(t, measurementID)
	})

	t.Run("test add plant image", func(t *testing.T) {
		plantImageID = testUploadPlantImage(t, plantID)
		if plantImageID == 0 {
			t.Fatal("Failed to create test plant image")
		}
	})

	t.Run("validate image and get path", func(t *testing.T) {
		resp, err := Client.Get(BaseURL + "/plant/" + strconv.Itoa(plantID))
		if err != nil {
			t.Fatalf("Failed to GET /plant/%d: %v", plantID, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200 OK from /plant/%d, got %d", plantID, resp.StatusCode)
		}

		// Read the response body
		body := new(bytes.Buffer)
		if _, err := io.Copy(body, resp.Body); err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
		bodyString := body.String()
		//Look for src="/uploads/plants/" to identify correct image on page
		//Parse src to imagePath
		startIndex := strings.Index(bodyString, "src=\"/uploads/plants/")
		if startIndex == -1 {
			t.Fatal("Image path not found in response body")
		}
		startIndex += len("src=\"")
		endIndex := strings.Index(bodyString[startIndex:], "\"")
		if endIndex == -1 {
			t.Fatal("End of image path not found in response body")
		}
		imagePath = bodyString[startIndex : startIndex+endIndex]
		if imagePath == "" {
			t.Fatal("Image path is empty")
		}
		t.Logf("Image path: %s", imagePath)
	})

	t.Run("test decorate plant image", func(t *testing.T) {
		decorateTestPlantImage(t, imagePath)
	})
	/*
		t.Run("test delete plant image", func(t *testing.T) {
			deleteTestPlantImage(t, plantImageID)
		})
		t.Run("update plant status", func(t *testing.T) {
			updateTestPlantStatus(t, plantID, "Updated Plant Status")
		})
	*/
	//Main Page
	//Plants List Page
	//Strains List Page
	//Graphs
	//Living / Harvested / Dead
	//Sensors / Add / Link / Edit / Delete
	//Plant Status CRUD
	//Scanning
	//Logo Upload
	//Multi Activity
	//Guest Mode

}

func decorateTestPlantImage(t *testing.T, imagePath string) {
	LoginAsAdmin(t, "newpass123")

	// Step 1: Parse Path
	//const imagePath = new URL(fullImagePath).pathname.replace(/^\//, "");
	imagePathCleaned := strings.TrimPrefix(imagePath, "/")

	// Step 2: Build the decoration request
	payload := map[string]string{
		"imagePath": imagePathCleaned,
		//"imagePath":   "uploads/plants/plant_1_image_0_1743873182788088000.jpg",
		"text1":       "Strain: Tester",
		"text2":       "Age: 14d",
		"text1Corner": "top-left",
		"text2Corner": "bottom-right",
		"logo":        "", // assuming "none" is valid
		"font":        "fonts/Anton-Regular.ttf",
		"textColor":   "#ffffff",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", BaseURL+"/decorateImage", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to build decorateImage request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("decorateImage request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from decorateImage, got %d", resp.StatusCode)
	}

	var result struct {
		Success    bool   `json:"success"`
		OutputPath string `json:"outputPath"`
		Error      string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse decorateImage response: %v", err)
	}

	if !result.Success {
		t.Fatalf("Decoration failed: %s", result.Error)
	}
	if result.OutputPath == "" {
		t.Fatalf("No output path returned in decoration response")
	}

	t.Logf("Decorated image written to: %s", result.OutputPath)
}

func deleteTestMeasurement(t *testing.T, measurementID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/plantMeasurement/delete/"+strconv.Itoa(measurementID), nil)
	if err != nil {
		t.Fatalf("Failed to build measurement DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send measurement DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /plantMeasurement/%d, got %d", measurementID, resp.StatusCode)
	}
}

func editTestMeasurement(t *testing.T, measurementID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"id":    measurementID,
		"date":  "2023-10-02",
		"value": 20,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/plantMeasurement/edit", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build measurement PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send measurement PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /plantMeasurement/edit, got %d", resp.StatusCode)
	}
}

func testAddMeasurement(t *testing.T, plantID int) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"plant_id":  plantID,
		"metric_id": 1,
		"value":     15,
		"date":      "2023-10-01",
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/plantMeasurement", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build measurement POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send measurement POST request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /plantMeasurement, got %d", resp.StatusCode)
	}
	var result struct {
		ID       int     `json:"plant_id"` // Now using int
		metricID int     `json:"metric_id"`
		value    float64 `json:"value"`
		date     string  `json:"date"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /plantMeasurement: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Measurement ID missing or zero in response")
	}
	t.Logf("Created test measurement: %d", result.ID)
	return result.ID
}

func deleteTestPlantActivity(t *testing.T, plantActivityID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/plantActivity/delete/"+strconv.Itoa(plantActivityID), nil)
	if err != nil {
		t.Fatalf("Failed to build plant activity DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send plant activity DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /plantActivity/%d, got %d", plantActivityID, resp.StatusCode)
	}
}

func editTestPlantActivity(t *testing.T, plantActivityID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"id":          plantActivityID,
		"activity_id": 1,
		"date":        "2023-10-02",
		"note":        "Updated test notes",
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/plantActivity/edit", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build plant activity PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send plant activity PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /plantActivity/edit, got %d", resp.StatusCode)
	}
}

func addPlantActivity(t *testing.T, plantID int) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"plant_id":    plantID,
		"activity_id": 1,
		"date":        "2023-10-01",
		"note":        "Test notes",
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/plantActivity", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build plant activity POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send plant activity POST request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /plant-activities, got %d", resp.StatusCode)
	}
	var result struct {
		ID         int    `json:"plant_id"`
		activityID int    `json:"activity_id"`
		date       string `json:"date"`
		note       string `json:"note"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /plant-activities: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Plant activity ID missing or zero in response")
	}
	t.Logf("Created test plant activity: %d", result.ID)
	return result.ID
}

func deleteTestPlant(t *testing.T, plantID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/plant/delete/"+strconv.Itoa(plantID), nil)
	if err != nil {
		t.Fatalf("Failed to build plant DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send plant DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /plants/%d, got %d", plantID, resp.StatusCode)
	}
}

func editTestPlant(t *testing.T, plantID int, plantName string, strainID int, zoneID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"plant_id":          plantID,
		"plant_name":        plantName,
		"plant_description": "Test description",
		"status_id":         2,
		"date":              "2023-10-01",
		"strain_id":         strainID,
		"zone_id":           zoneID,
		"clone":             false,
		"start_date":        "2023-10-01",
		"harvest_weight":    0,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/plant", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build plant PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send plant PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 201 OK from /plant, got %d", resp.StatusCode)
	}
}

func testAddPlant(t *testing.T, plantName string, strainID int, zoneID int) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"name":                 plantName,
		"strain_id":            strainID,
		"zone_id":              zoneID,
		"status_id":            1,
		"date":                 "2023-10-01",
		"clone":                0,
		"parent_id":            0,
		"decrement_seed_count": false,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/plants", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build plant POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send plant POST request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /plants, got %d", resp.StatusCode)
	}
	var result struct {
		ID      int    `json:"id"`
		message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /plants: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Plant ID missing or zero in response")
	}
	t.Logf("Created test plant: %d", result.ID)
	return result.ID

}

func deleteTestStrain(t *testing.T, strainID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/strains/"+strconv.Itoa(strainID), nil)
	if err != nil {
		t.Fatalf("Failed to build strain DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send strain DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /strains/%d, got %d", strainID, resp.StatusCode)
	}
}

func editTestStrain(t *testing.T, strainID int, breederID int, strainName string) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"name":        strainName,
		"breeder_id":  breederID,
		"indica":      50,
		"sativa":      50,
		"autoflower":  false,
		"seed_count":  5,
		"description": "Test description",
		"cycle_time":  84,
		"url":         "http://leafly.com/strains/test-strain",
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", BaseURL+"/strains/"+strconv.Itoa(strainID), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build strain PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send strain PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /strains/%d, got %d", strainID, resp.StatusCode)
	}
}

func testAddStrain(t *testing.T, strainName string, breederID int) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"name":        strainName,
		"breeder_id":  breederID,
		"indica":      50,
		"sativa":      50,
		"autoflower":  false,
		"seed_count":  5,
		"description": "Test description",
		"cycle_time":  84,
		"url":         "http://leafly.com/strains/test-strain",
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/strains", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build strain POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send strain POST request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /strains, got %d", resp.StatusCode)
	}
	var result struct {
		ID      int    `json:"id"`
		message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /strains: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Strain ID missing or zero in response")
	}
	t.Logf("Created test strain: %d", result.ID)
	return result.ID
}

func deleteTestBreeder(t *testing.T, breederID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/breeders/"+strconv.Itoa(breederID), nil)
	if err != nil {
		t.Fatalf("Failed to build breeder DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send breeder DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /breeders/%d, got %d", breederID, resp.StatusCode)
	}
}

func editTestBreeder(t *testing.T, breederID int, breederName string) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"breeder_name": breederName,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", BaseURL+"/breeders/"+strconv.Itoa(breederID), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build breeder PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send breeder PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /breeders/%d, got %d", breederID, resp.StatusCode)
	}
}

func testBreederAppearsInSettings(t *testing.T, breederID int) {
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

	// Look for a known unique piece of the breeder we added
	if !strings.Contains(html, "Test Breeder") {
		t.Errorf("Expected breeder name 'Test Breeder' to appear in /settings page HTML")
	}
	if !strings.Contains(html, strconv.Itoa(breederID)) {
		t.Errorf("Expected breeder ID %d to appear in /settings page HTML", breederID)
	}

}

func testAddBreeder(t *testing.T, breederName string) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"breeder_name": breederName,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/breeders", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build breeder POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send breeder POST request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /breeders, got %d", resp.StatusCode)
	}
	var result struct {
		ID int `json:"id"` // Now using int
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /breeders: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Breeder ID missing or zero in response")
	}
	t.Logf("Created test breeder: %d", result.ID)
	return result.ID
}

func deleteTestMetric(t *testing.T, metricID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/metrics/"+strconv.Itoa(metricID), nil)
	if err != nil {
		t.Fatalf("Failed to build metric DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send metric DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /metrics/%d, got %d", metricID, resp.StatusCode)
	}
}

func editTestMetric(t *testing.T, metricID int, metricName string) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"metric_name": metricName,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", BaseURL+"/metrics/"+strconv.Itoa(metricID), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build metric PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send metric PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /metrics/%d, got %d", metricID, resp.StatusCode)
	}
}

func testMetricAppearsInSettings(t *testing.T, metricID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
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
	// Look for a known unique piece of the metric we added
	if !strings.Contains(html, "Test Metric") {
		t.Errorf("Expected metric name 'Test Metric' to appear in /settings page HTML")
	}
}

func testAddMetric(t *testing.T, metricName string) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"metric_name": metricName,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/metrics", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build metric POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send metric POST request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /metrics, got %d", resp.StatusCode)
	}
	var result struct {
		ID int `json:"id"` // Now using int
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /metrics: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Metric ID missing or zero in response")
	}
	t.Logf("Created test metric: %d", result.ID)
	return result.ID
}

func deleteTestActivity(t *testing.T, activityID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/activities/"+strconv.Itoa(activityID), nil)
	if err != nil {
		t.Fatalf("Failed to build activity DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send activity DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /activities/%d, got %d", activityID, resp.StatusCode)
	}
	// Check if the activity is actually deleted
	resp, err = Client.Get(BaseURL + "/activities/" + strconv.Itoa(activityID))
	if err != nil {
		t.Fatalf("Failed to GET /activities/%d: %v", activityID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected 404 Not Found from /activities/%d after deletion, got %d", activityID, resp.StatusCode)
	}
}

func editTestActivity(t *testing.T, activityID int, activityName string) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"activity_name": activityName,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", BaseURL+"/activities/"+strconv.Itoa(activityID), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build activity PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send activity PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /activities/%d, got %d", activityID, resp.StatusCode)
	}
}

func testActivityAppearsInSettings(t *testing.T, activityID int) {
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

	// Look for a known unique piece of the activity we added
	if !strings.Contains(html, "Test Activity") {
		t.Errorf("Expected activity name 'Test Activity' to appear in /settings page HTML")
	}
	if !strings.Contains(html, strconv.Itoa(activityID)) {
		t.Errorf("Expected activity ID %d to appear in /settings page HTML", activityID)
	}
}

func testAddActivity(t *testing.T, activityName string) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"activity_name": activityName,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", BaseURL+"/activities", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build activity POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send activity POST request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from /activities, got %d", resp.StatusCode)
	}
	var result struct {
		ID int `json:"id"` // Now using int
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /activities: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Activity ID missing or zero in response")
	}
	t.Logf("Created test activity: %d", result.ID)
	return result.ID
}

func deleteTestStream(t *testing.T, streamID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/streams/"+strconv.Itoa(streamID), nil)
	if err != nil {
		t.Fatalf("Failed to build stream DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send stream DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /streams/%d, got %d", streamID, resp.StatusCode)
	}
}

func editTestStream(t *testing.T, streamID int, streamName string, zoneID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	payload := map[string]any{
		"stream_name": streamName,
		"id":          streamID,
		"url":         "http://example.com/stream.m3u8",
		"visible":     true,
		"zone_id":     zoneID,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", BaseURL+"/streams/"+strconv.Itoa(streamID), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build stream PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send stream PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /streams/%d, got %d", streamID, resp.StatusCode)
	}
}

func deleteTestZone(t *testing.T, zoneID int) {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
	req, err := http.NewRequest("DELETE", BaseURL+"/zones/"+strconv.Itoa(zoneID), nil)
	if err != nil {
		t.Fatalf("Failed to build zone DELETE request: %v", err)
	}
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send zone DELETE request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /zones/%d, got %d", zoneID, resp.StatusCode)
	}

}

func testInitLoginFlow(t *testing.T) {
	t.Run("login", func(t *testing.T) {
		LoginAsAdmin(t, "isley")
	})

	t.Run("change password", func(t *testing.T) {
		form := url.Values{}
		form.Set("new_password", "newpass123")
		form.Set("confirm_password", "newpass123")

		PostFormExpectRedirect(t, "/change-password", "/", form)
	})
}

func testAccessProtectedRoute(t *testing.T) {
	resp, err := Client.Get(BaseURL + "/settings")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK on protected / route, got %d", resp.StatusCode)
	}
}

func testDeniedProtectedRoute(t *testing.T) {
	resp, err := Client.Get(BaseURL + "/settings")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("Expected 302 Found on protected / route, got %d", resp.StatusCode)
	}
}

func testLogout(t *testing.T) {
	resp, err := Client.Get(BaseURL + "/logout")
	if err != nil {
		t.Fatalf("Logout request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Fatalf("Expected redirect to /login after logout, got status %d and Location %s",
			resp.StatusCode, resp.Header.Get("Location"))
	}
}

func testLoginWithNewPassword(t *testing.T) {
	t.Run("login", func(t *testing.T) {
		LoginAsAdmin(t, "newpass123")
	})
}

func editTestZone(t *testing.T, zoneID int, zoneName string) {
	LoginAsAdmin(t, "newpass123")
	payload := map[string]any{
		"zone_name": zoneName,
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("PUT", BaseURL+"/zones/"+strconv.Itoa(zoneID), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to build zone PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send zone PUT request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK from /zones/%d, got %d", zoneID, resp.StatusCode)
	}
}

func createTestZone(t *testing.T, zoneName string) int {
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test
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
		ID int `json:"id"` // Now using int
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response from /zones: %v", err)
	}

	if result.ID == 0 {
		t.Fatalf("Zone ID missing or zero in response")
	}

	t.Logf("Created test zone: %d", result.ID)
	return result.ID
}

func testAddStream(t *testing.T, zoneID int) int {
	// Ensure we're logged in with an admin session
	LoginAsAdmin(t, "newpass123") // assumes this was set in prior test

	// Convert zoneID to string
	strZoneID := strconv.Itoa(zoneID)
	payload := map[string]any{
		"stream_name": "Test Stream 1",
		"zone_id":     strZoneID,
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

	var result struct {
		ID int `json:"id"` // Now using int
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if result.ID == 0 {
		t.Fatalf("Stream ID missing or zero in response")
	}
	t.Logf("Created test stream: %d", result.ID)
	return result.ID
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

func testUploadPlantImage(t *testing.T, plantID int) int {
	LoginAsAdmin(t, "newpass123")

	// Step 2: Create multipart form body
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add image
	imageWriter, err := writer.CreateFormFile("images[]", "test.png")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	imageBytes := testPNG()
	if imageBytes == nil {
		t.Fatalf("Failed to load test image")
	}
	_, err = imageWriter.Write(imageBytes)
	if err != nil {
		t.Fatalf("Failed to write image: %v", err)
	}

	// Add description
	_ = writer.WriteField("descriptions[]", "Test image description")

	// Add date
	_ = writer.WriteField("dates[]", "2025-04-04")

	writer.Close()

	// Step 3: Send request
	url := fmt.Sprintf("%s/plant/%d/images/upload", BaseURL, plantID)
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("Image upload request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected 200 or 201 from image upload, got %d", resp.StatusCode)
	}
	//Returns an array of ids, parse for the response
	var result struct {
		IDs []int `json:"ids"` // Now using int
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if len(result.IDs) == 0 {
		t.Fatalf("Image ID missing or zero in response")
	}
	t.Logf("Uploaded test image: %d", result.IDs[0])
	return result.IDs[0]
}

func testPNG() []byte {
	projectRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	// Open the image file
	// /web/static/img/placeholder.png
	filePath := filepath.Join(projectRoot, "web", "static", "img", "placeholder.png")
	file, err := os.Open(filePath)
	if err != nil {
		panic("failed to open image: " + err.Error())
	}
	defer file.Close()
	imgBytes, err := io.ReadAll(file)
	if err != nil {
		panic("failed to read image: " + err.Error())
	}
	return imgBytes
}
