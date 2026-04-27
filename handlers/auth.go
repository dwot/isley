package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"isley/logger"
	"isley/utils"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Login rate limiting
// ---------------------------------------------------------------------------

var (
	loginAttempts   = make(map[string][]time.Time)
	loginAttemptsMu sync.Mutex

	// SecureCookies controls the Secure flag on session cookies.
	// Set via the ISLEY_SECURE_COOKIES environment variable.
	SecureCookies = strings.EqualFold(os.Getenv("ISLEY_SECURE_COOKIES"), "true")
)

// ResetLoginAttempts clears the in-memory rate-limiter map. It exists so
// that tests touching the /login route can isolate themselves from
// other tests in the same process — the underlying map is shared
// global state (Phase 7 in docs/TEST_PLAN.md will replace it with an
// instance-scoped store). Production code does not call this.
func ResetLoginAttempts() {
	loginAttemptsMu.Lock()
	loginAttempts = make(map[string][]time.Time)
	loginAttemptsMu.Unlock()
}

// IsLoginRateLimited returns true if the given IP has exceeded 5 login
// attempts within the last minute.
func IsLoginRateLimited(ip string) bool {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	now := time.Now()
	cutoff := now.Add(-time.Minute)
	attempts := loginAttempts[ip]
	var recent []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	loginAttempts[ip] = recent
	return len(recent) > MaxLoginAttempts
}

// ---------------------------------------------------------------------------
// CSRF token generation
// ---------------------------------------------------------------------------

// GenerateCSRFToken creates a cryptographically random hex token.
func GenerateCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		logger.Log.WithError(err).Error("Failed to generate CSRF token")
		return ""
	}
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Password validation
// ---------------------------------------------------------------------------

// ValidatePasswordComplexity checks that a password meets minimum security
// requirements. Returns an empty string on success or an error message.
func ValidatePasswordComplexity(password string) string {
	if len(password) < MinPasswordLength {
		return "Password must be at least 8 characters long"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Auth handlers
// ---------------------------------------------------------------------------

// HandleLogin processes the POST /login form submission.
func HandleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	remember := c.PostForm("remember")

	db := DBFromContext(c)

	storedUsername, _ := GetSetting(db, "auth_username")
	storedPasswordHash, _ := GetSetting(db, "auth_password")
	forcePasswordChange, _ := GetSetting(db, "force_password_change")

	if username != storedUsername || !utils.CheckPasswordHash(password, storedPasswordHash) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		csrfToken, _ := c.Get("csrf_token")
		c.HTML(http.StatusUnauthorized, "views/login.html", gin.H{
			"Error":           "Invalid username or password",
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       csrfToken,
		})
		return
	}

	session := sessions.Default(c)

	// SECURITY: Regenerate session to prevent session fixation attacks.
	// Clear the old session data and generate a new CSRF token.
	session.Clear()
	session.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		Secure:   SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	// If the user checked 'remember', extend session MaxAge to 14 days
	if remember == "on" || remember == "true" {
		session.Options(sessions.Options{
			Path:     "/",
			MaxAge:   14 * 24 * 60 * 60, // 14 days
			HttpOnly: true,
			Secure:   SecureCookies,
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Generate a fresh CSRF token for the new session
	session.Set("csrf_token", GenerateCSRFToken())
	session.Set("logged_in", true)
	session.Set("force_password_change", forcePasswordChange == "true")

	// Store current session version so it can be validated later
	sessionVersion, _ := GetSetting(db, "session_version")
	session.Set("session_version", sessionVersion)
	session.Save()

	if forcePasswordChange == "true" {
		c.Redirect(http.StatusFound, "/change-password")
		return
	}

	c.Redirect(http.StatusFound, "/")
}

// HandleLogout clears the session and redirects to the login page.
func HandleLogout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/login")
}

// HandleChangePassword processes the POST /change-password form submission.
func HandleChangePassword(c *gin.Context) {
	newPassword := c.PostForm("new_password")
	confirmPassword := c.PostForm("confirm_password")

	if newPassword != confirmPassword {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		csrfToken, _ := c.Get("csrf_token")
		c.HTML(http.StatusBadRequest, "views/change-password.html", gin.H{
			"Error":           "Passwords do not match",
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       csrfToken,
		})
		return
	}

	// SECURITY: Enforce password complexity requirements
	if errMsg := ValidatePasswordComplexity(newPassword); errMsg != "" {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		csrfToken, _ := c.Get("csrf_token")
		c.HTML(http.StatusBadRequest, "views/change-password.html", gin.H{
			"Error":           errMsg,
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       csrfToken,
		})
		return
	}

	hashedPassword, _ := utils.HashPassword(newPassword)

	db := DBFromContext(c)
	store := ConfigStoreFromContext(c)

	UpdateSetting(db, store, "auth_password", hashedPassword)
	UpdateSetting(db, store, "force_password_change", "false")

	// SECURITY: Bump session version to invalidate all other sessions.
	// Only the current session gets the new version, so all others become stale.
	newVersion := fmt.Sprintf("%d", time.Now().UnixNano())
	UpdateSetting(db, store, "session_version", newVersion)

	session := sessions.Default(c)
	session.Set("force_password_change", false)
	session.Set("session_version", newVersion)
	session.Save()

	c.Redirect(http.StatusFound, "/")
}

// ---------------------------------------------------------------------------
// Auth middleware
// ---------------------------------------------------------------------------

// AuthMiddlewareApi returns middleware that validates either an X-API-KEY
// header or an active session before allowing access to API routes.
func AuthMiddlewareApi() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-KEY")
		session := sessions.Default(c)
		loggedIn := session.Get("logged_in")

		if apiKey != "" {
			// Get stored (hashed) API key from settings
			db := DBFromContext(c)

			var storedAPIKey string
			err := db.QueryRow("SELECT value FROM settings WHERE name = 'api_key'").Scan(&storedAPIKey)
			if err != nil {
				logger.Log.WithError(err).Error("Error retrieving API key from database")
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Could not validate API key",
				})
				return
			}

			// Validate the incoming key against the stored hash.
			// CheckAPIKey handles bcrypt (preferred), legacy SHA-256, and
			// plaintext matches for backward compatibility.
			match, legacy := CheckAPIKey(apiKey, storedAPIKey)
			if !match {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "Invalid API key",
				})
				return
			}

			// Auto-upgrade legacy keys (SHA-256 or plaintext) to bcrypt.
			if legacy {
				if newHash := HashAPIKey(apiKey); newHash != "" {
					UpdateSetting(db, ConfigStoreFromContext(c), "api_key", newHash)
					logger.Log.Info("Auto-upgraded legacy API key to bcrypt")
				}
			}

		} else if loggedIn == nil || !loggedIn.(bool) {
			// If no API key is provided, check session for logged_in status
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "API key or session required",
			})
			return
		}

		c.Next()
	}
}

// AuthMiddleware returns middleware that enforces session-based authentication
// for browser routes. Redirects to /login if not authenticated or if the
// session version is stale (e.g. after a password change).
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		loggedIn := session.Get("logged_in")

		if loggedIn == nil || !loggedIn.(bool) {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// SECURITY: Validate session version to enforce invalidation on password change
		db := DBFromContext(c)
		dbVersion, _ := GetSetting(db, "session_version")
		sessVersion, _ := session.Get("session_version").(string)
		if dbVersion != "" && sessVersion != dbVersion {
			session.Clear()
			session.Save()
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Next()
	}
}

// ForcePasswordChangeMiddleware returns middleware that redirects to
// /change-password if the session flag is set, except when the user is
// already on that page.
func ForcePasswordChangeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		forcePasswordChange := session.Get("force_password_change")

		// Allow access to /change-password (both GET and POST) if force password change is required
		if forcePasswordChange != nil && forcePasswordChange.(bool) {
			if c.FullPath() != "/change-password" {
				c.Redirect(http.StatusFound, "/change-password")
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
