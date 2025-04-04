package integration

import (
	"net/http"
	"net/url"
	"testing"
)

func LoginAsAdmin(t *testing.T, password string) {
	t.Helper()

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", password)

	resp, err := Client.PostForm(BaseURL+"/login", form)
	if err != nil {
		t.Fatalf("LoginAsAdmin failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("Expected login redirect, got %d", resp.StatusCode)
	}
}
