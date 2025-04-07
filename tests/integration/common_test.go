package integration

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq" // Make sure this is in your imports
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
	appCmd     *exec.Cmd
	Client     *http.Client
	BaseURL    = "http://localhost:8080"
	DBFile     string
	testDBType string
)

func TestMain(m *testing.M) {
	testDBType = os.Getenv("ISLEY_TEST_DB")
	if testDBType == "" {
		testDBType = "sqlite"
	}
	setupApp()
	code := m.Run()
	teardownApp()
	os.Exit(code)
}

func setupApp() {
	projectRoot, _ := filepath.Abs(filepath.Join("..", ".."))
	binary := "isley"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	binaryPath, _ := filepath.Abs(filepath.Join("..", "..", binary))

	envVars := os.Environ()
	envVars = append(envVars, "ISLEY_PORT=8080")
	if testDBType == "sqlite" {
		setupSQLite(projectRoot, &envVars)
	} else if testDBType == "postgres" {
		setupPostgres(&envVars)
	} else {
		panic("unsupported ISLEY_TEST_DB value: " + testDBType)
	}

	appCmd = exec.Command(binaryPath)
	appCmd.Env = envVars
	appCmd.Dir = projectRoot
	appCmd.Stdout = os.Stdout
	appCmd.Stderr = os.Stderr
	if err := appCmd.Start(); err != nil {
		panic("failed to start app: " + err.Error())
	}

	waitForAppReady()

	jar, _ := cookiejar.New(nil)
	Client = &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func setupSQLite(projectRoot string, envVars *[]string) {
	dbDir := filepath.Join(projectRoot, "tmp")
	DBFile = filepath.Join(dbDir, "test.db")

	os.MkdirAll(dbDir, 0755)
	_ = os.Remove(DBFile)
	_ = os.Remove(DBFile + "-shm")
	_ = os.Remove(DBFile + "-wal")

	*envVars = append(*envVars,
		"ISLEY_DB_DRIVER=sqlite",
		"ISLEY_DB_FILE="+DBFile,
	)
}

func setupPostgres(envVars *[]string) {
	// Read values from env or defaults
	host := os.Getenv("ISLEY_TEST_DB_HOST")
	port := os.Getenv("ISLEY_TEST_DB_PORT")
	user := os.Getenv("ISLEY_TEST_DB_USER")
	pass := os.Getenv("ISLEY_TEST_DB_PASSWORD")
	name := os.Getenv("ISLEY_TEST_DB_NAME")

	if host == "" || port == "" || user == "" || name == "" {
		panic("missing one or more Postgres env vars")
	}

	*envVars = append(*envVars,
		"ISLEY_DB_DRIVER=postgres",
		"ISLEY_DB_HOST="+host,
		"ISLEY_DB_PORT="+port,
		"ISLEY_DB_USER="+user,
		"ISLEY_DB_PASSWORD="+pass,
		"ISLEY_DB_NAME="+name,
	)

	// Clear tables before tests (basic version)
	clearPostgresDB(host, port, user, pass, name)
}

func clearPostgresDB(host, port, user, pass, dbname string) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbname)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic("failed to connect to Postgres: " + err.Error())
	}
	defer db.Close()

	// Drop all user-defined tables and sequences in current schema
	_, err = db.Exec(`
		DO $$ DECLARE
			r RECORD;
		BEGIN
			-- Drop tables
			FOR r IN (
				SELECT tablename
				FROM pg_tables
				WHERE schemaname = current_schema()
				  AND tablename NOT LIKE 'pg_%'
				  AND tablename NOT LIKE 'sql_%'
			) LOOP
				EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE';
			END LOOP;

			-- Drop sequences
			FOR r IN (
				SELECT sequence_name
				FROM information_schema.sequences
				WHERE sequence_schema = current_schema()
			) LOOP
				EXECUTE 'DROP SEQUENCE IF EXISTS ' || quote_ident(r.sequence_name) || ' CASCADE';
			END LOOP;
		END $$;
	`)
	if err != nil {
		panic("failed to drop tables/sequences in Postgres: " + err.Error())
	}
}

func teardownApp() {
	if appCmd != nil && appCmd.Process != nil {
		_ = appCmd.Process.Kill()
		_, _ = appCmd.Process.Wait()
	}
	if testDBType == "sqlite" {
		_ = os.Remove(DBFile)
		_ = os.Remove(DBFile + "-shm")
		_ = os.Remove(DBFile + "-wal")
		_ = os.RemoveAll(filepath.Join("..", "..", "tmp"))
	}
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
