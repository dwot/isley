package integration

import (
	"net/http"
	"net/url"
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
	t.Run("logout2", testLogout)
	/*
		Additional Tests to be Designed and Implemented:
		Login & Perms
			1. Test invalid login credentials

		Settings Page
			1. Test Guest Mode
			2. Test AC Infinity Sensor Setup
			3. Test EcoWitt Sensor Setup
			4. Zones CRUD
			5. Activities CRUD
			6. Metrics CRUD
			7. Breeders CRUD

		Strains Page
			Add Strain
			Edit Strain
			Delete Strain

		Sensors Page
		Plants Page
		Plant Page
	*/
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
