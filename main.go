package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"isley/config"
	"isley/handlers"
	"isley/logger"
	"isley/model"
	"isley/routes"
	"isley/utils"
	"isley/watcher"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"github.com/sirupsen/logrus"
)

//go:embed model/migrations/sqlite/*.sql model/migrations/postgres/*.sql web/templates/**/*.html web/static/**/* utils/fonts/* VERSION
var embeddedFiles embed.FS

var (
	loginAttempts   = make(map[string][]time.Time)
	loginAttemptsMu sync.Mutex
)

func isLoginRateLimited(ip string) bool {
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
	return len(recent) > 5
}

// generateCSRFToken creates a cryptographically random hex token.
func generateCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		logger.Log.WithError(err).Error("Failed to generate CSRF token")
		return ""
	}
	return hex.EncodeToString(b)
}

// CSRFMiddleware generates a CSRF token per session and validates it on
// state-changing requests. JSON API calls authenticated via X-API-KEY are
// exempt because API keys are not automatically attached by browsers.
func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		// Ensure every session has a CSRF token
		token, _ := session.Get("csrf_token").(string)
		if token == "" {
			token = generateCSRFToken()
			session.Set("csrf_token", token)
			session.Save()
		}

		// Make the token available to templates and handlers
		c.Set("csrf_token", token)

		// Only validate on state-changing methods
		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "DELETE" {
			// Exempt requests authenticated via API key (not browser-initiated)
			if c.GetHeader("X-API-KEY") != "" {
				c.Next()
				return
			}

			// Check form field first, then header (for AJAX)
			submitted := c.PostForm("csrf_token")
			if submitted == "" {
				submitted = c.GetHeader("X-CSRF-Token")
			}

			if subtle.ConstantTimeCompare([]byte(submitted), []byte(token)) != 1 {
				logger.Log.WithField("path", c.Request.URL.Path).Warn("CSRF token validation failed")
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "CSRF token invalid"})
				return
			}
		}

		c.Next()
	}
}

func main() {
	// Initialize logger
	logger.InitLogger()

	// Set version
	version := fmt.Sprintf("Isley %s", getVersion())
	logger.Log.Info("Starting application version:", version)

	// Define the port
	port := os.Getenv("ISLEY_PORT")
	if port == "" {
		port = "8080" // Default port if environment variable PORT is not set
	}

	model.MigrateDB()
	model.InitDB()
	model.RunStartupMaintenance()
	dbDriver := model.GetDriver()
	version = fmt.Sprintf("%s-%s", version, dbDriver)

	// Initialize translation service
	utils.Init("en")

	// Initialize default admin credentials if not present
	present, err := handlers.ExistsSetting("auth_username")
	if err != nil {
		logger.Log.WithError(err).Error("Error checking if default admin credentials are present")
	} else {
		if !present {
			handlers.UpdateSetting("auth_username", "admin")
			hashedPassword, _ := utils.HashPassword("isley")
			handlers.UpdateSetting("auth_password", hashedPassword)
			handlers.UpdateSetting("force_password_change", "true")
		}
	}

	// Load settings before pruning so config.SensorRetention is populated
	handlers.LoadSettings()

	if config.SensorRetention <= 0 {
		logger.Log.Warn("Sensor data retention is disabled (sensor_retention_days = 0). " +
			"Sensor data will grow indefinitely. Consider setting a retention period " +
			"(e.g. 90 days) in Settings to prevent unbounded database growth.")
	}

	// Start the PruneSensorData and wait for it to complete before starting the main watcher Watch function. This ensures that old sensor data is pruned before we start grabbing new data.
	if err := watcher.PruneSensorData(); err != nil {
		logger.Log.WithError(err).Error("Initial sensor data prune failed")
	} else {
		logger.Log.Info("Initial sensor data prune completed")
	}
	go watcher.Watch()

	// Default to release mode in production; override with GIN_MODE=debug
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Set up Gin router
	r := gin.New()

	// Trust only loopback and RFC-1918 private ranges so that
	// c.ClientIP() returns the real client IP behind a reverse proxy.
	// Users running without a proxy are unaffected.
	r.SetTrustedProxies([]string{
		"127.0.0.1",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"::1",
	})

	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithWriter(logger.AccessWriter))

	// Security headers
	r.Use(func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; "+
				"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com https://fonts.googleapis.com; "+
				"font-src 'self' data: https://fonts.gstatic.com https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; "+
				"img-src 'self' data: blob:; "+
				"connect-src 'self' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com")
		c.Next()
	})

	// inject currentPath into the context
	r.Use(func(c *gin.Context) {
		c.Set("currentPath", c.Request.URL.Path)
		c.Next()
	})

	funcMap := template.FuncMap{
		"upper":     strings.ToUpper, // Define the 'upper' function
		"lower":     strings.ToLower, // Define the 'lower' function for templates
		"hasPrefix": strings.HasPrefix,
		"default": func(val interface{}, def string) string {
			if str, ok := val.(string); ok && str != "" {
				return str
			}
			return def
		},
		"json": func(v interface{}) string {
			a, err := json.Marshal(v)
			if err != nil {
				logger.Log.WithError(err).Error("Error marshalling JSON")
				return ""
			}
			return string(a)
		},
		"formatDateTimeLocal": func(t time.Time) string {
			return t.Local().Format(utils.LayoutDateTimeLocal)
		},
		"formatDate": func(t time.Time) string {
			return t.Format(utils.LayoutDate)
		},
		"formatDateTime": func(t time.Time) string { return t.Format(utils.LayoutDateTime) },
		"formatDateISO": func(t time.Time) string {
			return t.Format(utils.LayoutDate)
		},
		"isZeroDate": func(t time.Time) bool {
			return utils.IsZeroDate(t)
		},
		"div": func(a, b int) int {
			return a / b
		},
		"toInt": func(value interface{}) int {
			switch v := value.(type) {
			case string:
				intVal, err := strconv.Atoi(v)
				if err != nil {
					logger.Log.WithFields(logrus.Fields{
						"input": v,
						"error": err,
					}).Error("Error converting string to int")
					return 0
				}
				return intVal
			case float64:
				return int(v)
			case int:
				return v
			default:
				logger.Log.WithField("input", value).Warn("Unhandled type in toInt conversion")
				return 0
			}
		},
		"preview": func(t string) string {
			if len(t) > 100 {
				return t[:100] + "..."
			}
			return t
		},
		"now": func() time.Time {
			return time.Now()
		},
		"markdownify": func(t string) template.HTML {
			// Render Markdown to HTML
			unsafe := blackfriday.Run([]byte(t))

			// Sanitize the HTML
			safe := bluemonday.UGCPolicy().SanitizeBytes(unsafe)

			// Return as template.HTML so it's not escaped again
			return template.HTML(safe)
		},
		"jsonify": func(v interface{}) template.HTML {
			a, err := json.Marshal(v)
			if err != nil {
				logger.Log.WithError(err).Error("Error marshalling JSON")
				return ""
			}
			return template.HTML(a)
		},
		"csrfToken": func() string {
			// Placeholder — overridden per-request by middleware via c.Set
			return ""
		},
		"linebreaks": func(s string) template.HTML {
			// Replace newlines with <br> tags and mark as safe HTML
			return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
		},
		"formatStringDate": func(s string) string {
			for _, layout := range []string{time.RFC3339, utils.LayoutDB, utils.LayoutDateTimeLocal, utils.LayoutDate} {
				if t, err := time.Parse(layout, s); err == nil {
					return t.Local().Format(utils.LayoutDateTime)
				}
			}
			return s
		},
		"formatStringDateOnly": func(s string) string {
			for _, layout := range []string{time.RFC3339, utils.LayoutDB, utils.LayoutDateTimeLocal, utils.LayoutDate} {
				if t, err := time.Parse(layout, s); err == nil {
					return t.Local().Format(utils.LayoutDate)
				}
			}
			return s
		},
	}

	// Attach FuncMap and ParseFS
	templ := template.Must(template.New("").Funcs(funcMap).ParseFS(embeddedFiles, "web/templates/**/*.html"))

	// Set HTML templates in Gin
	r.SetHTMLTemplate(templ)

	// Load settings (PollingInterval, ACIEnabled, etc.)
	handlers.LoadSettings()

	go watcher.Grab()

	r.Static("/uploads", "./uploads")

	r.GET("/static/*filepath", func(c *gin.Context) {
		filePath := fmt.Sprintf("web/static%s", c.Param("filepath"))
		data, err := embeddedFiles.ReadFile(filePath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		http.ServeContent(c.Writer, c.Request, filePath, time.Now(), strings.NewReader(string(data)))
	})

	r.GET("/fonts/*filepath", func(c *gin.Context) {
		filePath := fmt.Sprintf("utils/fonts%s", c.Param("filepath"))
		data, err := embeddedFiles.ReadFile(filePath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		http.ServeContent(c.Writer, c.Request, filePath, time.Now(), strings.NewReader(string(data)))
	})

	// Initialize session store
	sessionSecret := os.Getenv("ISLEY_SESSION_SECRET")
	if sessionSecret == "" {
		logger.Log.Warn("ISLEY_SESSION_SECRET not set; generating a random session key (sessions will not survive restart)")
		randomBytes := make([]byte, 32)
		if _, err := rand.Read(randomBytes); err != nil {
			logger.Log.WithError(err).Fatal("Failed to generate random session key")
		}
		sessionSecret = string(randomBytes)
	}
	store := cookie.NewStore([]byte(sessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("isley_session", store))

	// CSRF protection — must come after sessions middleware
	r.Use(CSRFMiddleware())

	// Database middleware — injects *sql.DB into Gin context so handlers can
	// retrieve it via handlers.DBFromContext(c) instead of calling model.GetDB().
	r.Use(func(c *gin.Context) {
		db, err := model.GetDB()
		if err != nil {
			logger.Log.WithError(err).Error("Database unavailable")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Database unavailable"})
			return
		}
		c.Set("db", db)
		c.Next()
	})

	// Public routes
	r.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		loggedIn := session.Get("logged_in")
		if loggedIn != nil && loggedIn.(bool) {
			c.Redirect(http.StatusFound, "/")
			return
		}

		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		csrfToken, _ := c.Get("csrf_token")
		c.HTML(http.StatusOK, "views/login.html", gin.H{
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       csrfToken,
		})
	})

	r.POST("/login", func(c *gin.Context) {
		if isLoginRateLimited(c.ClientIP()) {
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		handleLogin(c)
	})

	r.GET("/logout", func(c *gin.Context) {
		handleLogout(c)
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		// Open the favicon from the embedded filesystem
		faviconData, err := embeddedFiles.ReadFile("web/static/img/favicon.ico")
		if err != nil {
			c.String(500, "Failed to load favicon")
			return
		}

		// Write the favicon data to the response
		c.Data(200, "image/x-icon", faviconData)
	})
	r.GET("/health", func(c *gin.Context) {
		handleHealth(c)
	})

	guestMode := false
	if config.GuestMode == 1 {
		guestMode = true
	}

	if guestMode {
		routes.AddBasicRoutes(r.Group("/"), version)
	}

	protected := r.Group("/")
	protected.Use(AuthMiddleware())
	{
		protected.Use(ForcePasswordChangeMiddleware())

		protected.GET("/change-password", func(c *gin.Context) {
			lang := utils.GetLanguage(c)
			translations := utils.TranslationService.GetTranslations(lang)
			csrfToken, _ := c.Get("csrf_token")
			c.HTML(http.StatusOK, "views/change-password.html", gin.H{
				"lcl":             translations,
				"languages":       utils.AvailableLanguages,
				"currentLanguage": lang,
				"csrfToken":       csrfToken,
			})
		})

		protected.POST("/change-password", func(c *gin.Context) {
			handleChangePassword(c)
		})

		routes.AddProtectedRoutes(protected, version)

		if !guestMode {
			routes.AddBasicRoutes(protected, version)
		}
	}

	apiProtected := r.Group("/")
	apiProtected.Use(AuthMiddlewareApi())
	{
		routes.AddProtectedApiRoutes(apiProtected)
		routes.AddExternalApiRoutes(apiProtected)
	}

	// Start the server
	logger.Log.Fatal(r.Run(":" + port))
}

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
	logger.Log.Info("Health check passed")
}

func handleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")
	remember := c.PostForm("remember")

	storedUsername, _ := handlers.GetSetting("auth_username")
	storedPasswordHash, _ := handlers.GetSetting("auth_password")
	forcePasswordChange, _ := handlers.GetSetting("force_password_change")

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
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// If the user checked 'remember', extend session MaxAge to 14 days
	if remember == "on" || remember == "true" {
		session.Options(sessions.Options{
			Path:     "/",
			MaxAge:   14 * 24 * 60 * 60, // 14 days
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Generate a fresh CSRF token for the new session
	session.Set("csrf_token", generateCSRFToken())
	session.Set("logged_in", true)
	session.Set("force_password_change", forcePasswordChange == "true")

	// Store current session version so it can be validated later
	sessionVersion, _ := handlers.GetSetting("session_version")
	session.Set("session_version", sessionVersion)
	session.Save()

	if forcePasswordChange == "true" {
		c.Redirect(http.StatusFound, "/change-password")
		return
	}

	c.Redirect(http.StatusFound, "/")
}

func handleLogout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/login")
}

// validatePasswordComplexity checks that a password meets minimum security requirements.
func validatePasswordComplexity(password string) string {
	if len(password) < 8 {
		return "Password must be at least 8 characters long"
	}
	return ""
}

func handleChangePassword(c *gin.Context) {
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
	if errMsg := validatePasswordComplexity(newPassword); errMsg != "" {
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

	handlers.UpdateSetting("auth_password", hashedPassword)
	handlers.UpdateSetting("force_password_change", "false")

	// SECURITY: Bump session version to invalidate all other sessions.
	// Only the current session gets the new version, so all others become stale.
	newVersion := fmt.Sprintf("%d", time.Now().UnixNano())
	handlers.UpdateSetting("session_version", newVersion)

	session := sessions.Default(c)
	session.Set("force_password_change", false)
	session.Set("session_version", newVersion)
	session.Save()

	c.Redirect(http.StatusFound, "/")
}

func AuthMiddlewareApi() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-KEY")
		session := sessions.Default(c)
		loggedIn := session.Get("logged_in")

		if apiKey != "" {
			// Get stored (hashed) API key from settings
			db, err := model.GetDB()
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Database error",
				})
				return
			}

			var storedAPIKey string
			err = db.QueryRow("SELECT value FROM settings WHERE name = 'api_key'").Scan(&storedAPIKey)
			if err != nil {
				logger.Log.WithError(err).Error("Error retrieving API key from database")
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Could not validate API key",
				})
				return
			}

			// Hash the incoming key and compare against the stored hash.
			// Also accept a direct match for backward compatibility with
			// keys that were stored before hashing was introduced.
			incomingHash := sha256.Sum256([]byte(apiKey))
			incomingHashHex := hex.EncodeToString(incomingHash[:])

			hashMatch := subtle.ConstantTimeCompare([]byte(incomingHashHex), []byte(storedAPIKey)) == 1
			legacyMatch := subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedAPIKey)) == 1

			if !hashMatch && !legacyMatch {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "Invalid API key",
				})
				return
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

// Middleware to enforce authentication
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
		dbVersion, _ := handlers.GetSetting("session_version")
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

// Middleware to enforce password change
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

func getVersion() string {
	// Read the VERSION file from the embedded filesystem
	data, err := embeddedFiles.ReadFile("VERSION")
	if err != nil {
		return "dev" // fallback to "dev" for local builds
	}
	return strings.TrimSpace(string(data))
}
