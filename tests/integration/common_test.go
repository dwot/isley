package integration

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var (
	appCmd  *exec.Cmd
	Client  *http.Client
	BaseURL = "http://localhost:8080"
	DBFile  string
)

func TestMain(m *testing.M) {
	println(">>> TestMain running")
	setupApp()
	code := m.Run()
	teardownApp()
	os.Exit(code)
}

func setupApp() {
	println(">>> setupApp start")
	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic("failed to determine project root: " + err.Error())
	}

	dbDir := filepath.Join(projectRoot, "tmp")
	DBFile = filepath.Join(dbDir, "test.db")

	os.MkdirAll(dbDir, 0755)
	_ = os.Remove(DBFile)
	_ = os.Remove(DBFile + "-shm")
	_ = os.Remove(DBFile + "-wal")

	binary := "isley"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	binaryPath, _ := filepath.Abs(filepath.Join("..", "..", binary))

	appCmd = exec.Command(binaryPath)
	appCmd.Env = append(os.Environ(),
		"ISLEY_DB_DRIVER=sqlite",
		"ISLEY_DB_FILE="+DBFile,
		"ISLEY_PORT=8080",
	)
	appCmd.Dir = filepath.Join("..", "..")
	appCmd.Stdout = os.Stdout
	appCmd.Stderr = os.Stderr
	_ = appCmd.Start()

	waitForAppReady()

	jar, _ := cookiejar.New(nil)
	Client = &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	println(">>> setupApp end")
}

func teardownApp() {
	if appCmd != nil && appCmd.Process != nil {
		_ = appCmd.Process.Kill()
		_, _ = appCmd.Process.Wait()
	}
	_ = os.Remove(DBFile)
	_ = os.Remove(DBFile + "-shm")
	_ = os.Remove(DBFile + "-wal")
	_ = os.RemoveAll(filepath.Join("..", "..", "tmp"))
}

func waitForAppReady() {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(BaseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	panic("app not ready after timeout")
}

func PostFormExpectRedirect(t *testing.T, path, expectedLocation string, form url.Values) {
	t.Helper()

	req, err := http.NewRequest("POST", BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("Failed to build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := Client.Do(req)
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != expectedLocation {
		t.Fatalf("Expected redirect to %s from %s, got %d and Location %s",
			expectedLocation, path, resp.StatusCode, resp.Header.Get("Location"))
	}
}
