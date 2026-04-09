package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
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
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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

// CSRFMiddleware generates a CSRF token per session and validates it on
// state-changing requests. JSON API calls authenticated via X-API-KEY are
// exempt because API keys are not automatically attached by browsers.
func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		// Ensure every session has a CSRF token
		token, _ := session.Get("csrf_token").(string)
		if token == "" {
			token = handlers.GenerateCSRFToken()
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
	db, err := model.GetDB()
	if err != nil {
		logger.Log.WithError(err).Fatal("Failed to open database for credential init")
	}
	present, err := handlers.ExistsSetting(db, "auth_username")
	if err != nil {
		logger.Log.WithError(err).Error("Error checking if default admin credentials are present")
	} else {
		if !present {
			handlers.UpdateSetting(db, "auth_username", "admin")
			hashedPassword, _ := utils.HashPassword("isley")
			handlers.UpdateSetting(db, "auth_password", hashedPassword)
			handlers.UpdateSetting(db, "force_password_change", "true")
		}
	}

	// Load settings before pruning so config.SensorRetention is populated
	handlers.LoadSettings()

	if config.SensorRetention <= 0 {
		logger.Log.Warn("Sensor data retention is disabled (sensor_retention_days = 0). " +
			"Sensor data will grow indefinitely. Consider setting a retention period " +
			"(e.g. 90 days) in Settings to prevent unbounded database growth.")
	}

	// Create a cancellable context for graceful shutdown of background goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the PruneSensorData and wait for it to complete before starting the main watcher Watch function. This ensures that old sensor data is pruned before we start grabbing new data.
	if err := watcher.PruneSensorData(); err != nil {
		logger.Log.WithError(err).Error("Initial sensor data prune failed")
	} else {
		logger.Log.Info("Initial sensor data prune completed")
	}
	watcher.WG.Add(1)
	go watcher.Watch(ctx)

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

	// Security headers with per-request CSP nonce.
	//
	// Each response gets a unique nonce that is added to the CSP header and
	// made available to templates via {{ .cspNonce }}. Inline <script> and
	// <style> blocks can opt in by adding nonce="{{ .cspNonce }}".
	//
	// Migration plan (tracked in TODO.md):
	//   Phase 1 (done) — generate nonce and include 'nonce-…' alongside
	//     'unsafe-inline' so existing pages keep working while templates
	//     are incrementally updated.
	//   Phase 2 — add nonce="{{ .cspNonce }}" to each inline <script> block
	//     (8 blocks across footer, strains, plants, plant, sensors, settings,
	//     strain, strain-edit) and the 1 inline <style> in settings.
	//   Phase 3 — add 'nonce-...' to script-src and style-src in the CSP
	//     header (nonce is already generated and passed to templates).
	//     NOTE: adding a nonce causes CSP Level 3 browsers to ignore
	//     'unsafe-inline', so ALL inline scripts/styles must have nonce
	//     attributes BEFORE the nonce appears in the header.
	//   Phase 4 — refactor inline onclick handlers (16 total) to
	//     addEventListener in external JS files.
	//   Phase 5 — remove 'unsafe-inline' from script-src.
	//   Phase 6 — audit for 'unsafe-eval' usage (likely Chart.js or
	//     template literals) and remove if possible.
	r.Use(func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Generate a per-request nonce for CSP.
		nonceBytes := make([]byte, 16)
		_, _ = rand.Read(nonceBytes)
		nonce := fmt.Sprintf("%x", nonceBytes)
		c.Set("cspNonce", nonce)

		// Build connect-src and media-src dynamically so the HLS player
		// can reach user-configured stream servers (e.g. Owncast).
		connectSrc := "'self' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com"
		mediaSrc := "'self' blob:"
		for _, s := range config.Streams {
			if parsed, err := url.Parse(s.URL); err == nil && parsed.Host != "" {
				origin := parsed.Scheme + "://" + parsed.Host
				if !strings.Contains(connectSrc, origin) {
					connectSrc += " " + origin
				}
				if !strings.Contains(mediaSrc, origin) {
					mediaSrc += " " + origin
				}
			}
		}

		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-eval' 'nonce-"+nonce+"' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; "+
				"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com https://fonts.googleapis.com; "+
				"font-src 'self' data: https://fonts.gstatic.com https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; "+
				"img-src 'self' data: blob:; "+
				"worker-src 'self' blob:; "+
				"media-src "+mediaSrc+"; "+
				"connect-src "+connectSrc)
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
			if config.Timezone != "" {
				if loc, err := time.LoadLocation(config.Timezone); err == nil {
					return t.In(loc).Format(utils.LayoutDateTimeLocal)
				}
			}
			return t.Local().Format(utils.LayoutDateTimeLocal)
		},
		"formatDate": func(t time.Time) string {
			if config.Timezone != "" {
				if loc, err := time.LoadLocation(config.Timezone); err == nil {
					return t.In(loc).Format(utils.LayoutDate)
				}
			}
			return t.Format(utils.LayoutDate)
		},
		"formatDateTime": func(t time.Time) string {
			if config.Timezone != "" {
				if loc, err := time.LoadLocation(config.Timezone); err == nil {
					return t.In(loc).Format(utils.LayoutDateTime)
				}
			}
			return t.Format(utils.LayoutDateTime)
		},
		"formatDateISO": func(t time.Time) string {
			if config.Timezone != "" {
				if loc, err := time.LoadLocation(config.Timezone); err == nil {
					return t.In(loc).Format(utils.LayoutDate)
				}
			}
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
			case uint:
				return int(v)
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
			if config.Timezone != "" {
				if loc, err := time.LoadLocation(config.Timezone); err == nil {
					return time.Now().In(loc)
				}
			}
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

	watcher.WG.Add(1)
	go watcher.Grab(ctx)

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
		Secure:   handlers.SecureCookies,
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
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.POST("/login", func(c *gin.Context) {
		if handlers.IsLoginRateLimited(c.ClientIP()) {
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		handlers.HandleLogin(c)
	})

	r.GET("/logout", func(c *gin.Context) {
		handlers.HandleLogout(c)
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
	protected.Use(handlers.AuthMiddleware())
	{
		protected.Use(handlers.ForcePasswordChangeMiddleware())

		protected.GET("/change-password", func(c *gin.Context) {
			lang := utils.GetLanguage(c)
			translations := utils.TranslationService.GetTranslations(lang)
			csrfToken, _ := c.Get("csrf_token")
			c.HTML(http.StatusOK, "views/change-password.html", gin.H{
				"lcl":             translations,
				"languages":       utils.AvailableLanguages,
				"currentLanguage": lang,
				"csrfToken":       csrfToken,
				"cspNonce":        c.GetString("cspNonce"),
			})
		})

		protected.POST("/change-password", func(c *gin.Context) {
			handlers.HandleChangePassword(c)
		})

		routes.AddProtectedRoutes(protected, version)

		if !guestMode {
			routes.AddBasicRoutes(protected, version)
		}
	}

	apiProtected := r.Group("/")
	apiProtected.Use(handlers.AuthMiddlewareApi())
	{
		routes.AddProtectedApiRoutes(apiProtected)
		routes.AddExternalApiRoutes(apiProtected)
	}

	// Start the HTTP server in a goroutine so we can handle shutdown signals.
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.WithError(err).Fatal("HTTP server error")
		}
	}()

	logger.Log.WithField("port", port).Info("Server started")

	// Wait for interrupt signal (SIGINT or SIGTERM) to gracefully shut down.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info("Shutdown signal received, stopping gracefully...")

	// Cancel watcher goroutines and wait for them to finish.
	cancel()
	watcher.WG.Wait()
	logger.Log.Info("Background goroutines stopped")

	// Give the HTTP server a few seconds to finish in-flight requests.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Log.WithError(err).Error("HTTP server forced to shutdown")
	}

	logger.Log.Info("Server exited cleanly")
}

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
	logger.Log.Info("Health check passed")
}

func getVersion() string {
	// Read the VERSION file from the embedded filesystem
	data, err := embeddedFiles.ReadFile("VERSION")
	if err != nil {
		return "dev" // fallback to "dev" for local builds
	}
	return strings.TrimSpace(string(data))
}
