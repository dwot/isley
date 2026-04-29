package app

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"github.com/sirupsen/logrus"

	"isley/config"
	"isley/handlers"
	"isley/logger"
	"isley/utils"
)

// ResolvePathDefaults applies the documented defaults for the path
// fields on Config. Centralised so NewEngine, production main, and
// tests all see the same fallback rules: UploadDir → "uploads",
// StreamDir → <UploadDir>/streams, FrameDir → <StreamDir>, LogsDir →
// "logs". Returning a copy keeps the input Config immutable from the
// caller's perspective.
//
// Exported so production main can read the resolved FrameDir before
// constructing the watcher's grabber, and so tests/testutil can
// mirror the engine's resolved values onto its TestServer struct
// without duplicating the rules. NewEngine itself calls this — there
// is no scenario in which it makes sense to construct an engine with
// the unresolved Config.
func ResolvePathDefaults(cfg Config) Config {
	if cfg.UploadDir == "" {
		cfg.UploadDir = handlers.DefaultUploadDir
	}
	if cfg.StreamDir == "" {
		cfg.StreamDir = handlers.DefaultStreamDir(cfg.UploadDir)
	}
	if cfg.FrameDir == "" {
		cfg.FrameDir = cfg.StreamDir
	}
	if cfg.LogsDir == "" {
		cfg.LogsDir = handlers.DefaultLogsDir
	}
	return cfg
}

// NewEngine wires up the full Gin engine — middleware, templates, routes —
// and returns it ready to be served. NewEngine does not start background
// services or the HTTP listener; callers own those. The returned engine
// is safe to mount on httptest.NewServer in tests.
func NewEngine(cfg Config) (*gin.Engine, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("app.NewEngine: cfg.DB is required")
	}
	if cfg.Assets == nil {
		return nil, fmt.Errorf("app.NewEngine: cfg.Assets is required")
	}
	if len(cfg.SessionSecret) < 32 {
		return nil, fmt.Errorf("app.NewEngine: cfg.SessionSecret must be at least 32 bytes (got %d)", len(cfg.SessionSecret))
	}

	cfg = ResolvePathDefaults(cfg)

	configStore := cfg.ConfigStore
	if configStore == nil {
		configStore = config.NewStore()
	}

	r := gin.New()

	if len(cfg.TrustedProxies) > 0 {
		if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			return nil, fmt.Errorf("set trusted proxies: %w", err)
		}
	} else {
		// Explicitly trust no one when empty — Gin's default trusts all
		// proxies which is unsafe.
		_ = r.SetTrustedProxies(nil)
	}

	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithWriter(logger.AccessWriter))
	r.Use(securityHeadersMiddleware(configStore))
	r.Use(currentPathMiddleware())

	templ, err := parseTemplates(cfg.Assets, configStore)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	r.SetHTMLTemplate(templ)

	registerStaticRoutes(r, cfg.Assets)

	store := cookie.NewStore(cfg.SessionSecret)
	store.Options(sessions.Options{
		Path:     "/",
		HttpOnly: true,
		Secure:   cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("isley_session", store))
	r.Use(csrfMiddleware())
	r.Use(dbMiddleware(cfg.DB))
	r.Use(configStoreMiddleware(configStore))
	r.Use(pathDirsMiddleware(cfg.UploadDir, cfg.StreamDir, cfg.LogsDir))

	backupSvc := cfg.BackupService
	if backupSvc == nil {
		backupSvc = handlers.NewBackupService(cfg.DB, cfg.DataDir)
	}
	r.Use(backupServiceMiddleware(backupSvc))

	rateLimiterSvc := cfg.RateLimiterService
	if rateLimiterSvc == nil {
		rateLimiterSvc = handlers.NewRateLimiterService(nil, nil)
	}
	r.Use(rateLimiterServiceMiddleware(rateLimiterSvc))

	sensorCacheSvc := cfg.SensorCacheService
	if sensorCacheSvc == nil {
		sensorCacheSvc = handlers.NewSensorCacheService(
			groupedSensorTTLFromStore(configStore),
			0,
		)
	}
	r.Use(sensorCacheServiceMiddleware(sensorCacheSvc))

	registerPublicRoutes(r, cfg)
	registerProtectedRoutes(r, cfg)
	registerAPIRoutes(r)

	return r, nil
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

func securityHeadersMiddleware(store *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		nonceBytes := make([]byte, 16)
		if _, err := rand.Read(nonceBytes); err != nil {
			logger.Log.WithError(err).Error("Failed to generate CSP nonce")
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		nonce := fmt.Sprintf("%x", nonceBytes)
		c.Set("cspNonce", nonce)

		// Build connect-src and media-src dynamically so the HLS player
		// can reach user-configured stream servers (e.g. Owncast).
		connectSrc := "'self' https://cdn.jsdelivr.net https://cdnjs.cloudflare.com"
		mediaSrc := "'self' blob:"
		for _, s := range store.Streams() {
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
	}
}

func currentPathMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("currentPath", c.Request.URL.Path)
		c.Next()
	}
}

// csrfMiddleware mirrors main.CSRFMiddleware: per-session token, exempts
// X-API-KEY-authenticated requests, validates POST/PUT/DELETE.
func csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)

		token, _ := session.Get("csrf_token").(string)
		if token == "" {
			token = handlers.GenerateCSRFToken()
			session.Set("csrf_token", token)
			_ = session.Save()
		}
		c.Set("csrf_token", token)

		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "DELETE" {
			if c.GetHeader("X-API-KEY") != "" {
				c.Next()
				return
			}
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

// dbMiddleware injects a *sql.DB closed over by NewEngine into the Gin
// context. Handlers retrieve it via handlers.DBFromContext(c). This
// removes the dependency on the model package's global, which is what
// lets multiple test engines run side-by-side against different DBs.
func dbMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	}
}

// backupServiceMiddleware injects the per-engine *handlers.BackupService
// into the Gin context. Handlers retrieve it via
// handlers.BackupServiceFromContext(c). The service owns backup/restore
// status state that previously lived in handlers package globals.
func backupServiceMiddleware(svc *handlers.BackupService) gin.HandlerFunc {
	return func(c *gin.Context) {
		handlers.SetBackupServiceOnContext(c, svc)
		c.Next()
	}
}

// rateLimiterServiceMiddleware injects the per-engine
// *handlers.RateLimiterService into the Gin context. The ingest
// middleware and the /login handler resolve the active limiter from
// context, which is what lets every test construct its own service
// (and therefore call t.Parallel() without colliding on counter state).
func rateLimiterServiceMiddleware(svc *handlers.RateLimiterService) gin.HandlerFunc {
	return func(c *gin.Context) {
		handlers.SetRateLimiterServiceOnContext(c, svc)
		c.Next()
	}
}

// sensorCacheServiceMiddleware injects the per-engine
// *handlers.SensorCacheService into the Gin context. The chart and
// grouped-sensor handlers resolve the cache through this so every
// engine carries its own buckets — what unblocks t.Parallel() on the
// chart-exercising tests.
func sensorCacheServiceMiddleware(svc *handlers.SensorCacheService) gin.HandlerFunc {
	return func(c *gin.Context) {
		handlers.SetSensorCacheServiceOnContext(c, svc)
		c.Next()
	}
}

// groupedSensorTTLFromStore returns the closure NewSensorCacheService
// consults on each grouped-cache read. The TTL is PollingInterval/10
// seconds, mirroring the pre-Phase-4.2 calculation in
// GetGroupedSensorsWithLatestReading; reading off the live Store on
// every call lets a settings change take effect on the next cache miss
// without re-wiring the service.
func groupedSensorTTLFromStore(store *config.Store) func() time.Duration {
	return func() time.Duration {
		interval := handlers.DefaultPollingIntervalSeconds
		if store != nil {
			interval = store.PollingInterval()
		}
		if interval <= 0 {
			interval = handlers.DefaultPollingIntervalSeconds
		}
		return time.Duration(interval/10) * time.Second
	}
}

// configStoreMiddleware injects the per-engine *config.Store into the
// Gin context. Handlers retrieve it via
// handlers.ConfigStoreFromContext(c). The store owns all runtime-mutable
// configuration that previously lived in config package globals; making
// it per-engine is what lets handler tests run in parallel without
// colliding on global state.
func configStoreMiddleware(s *config.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		handlers.SetConfigStoreOnContext(c, s)
		c.Next()
	}
}

// pathDirsMiddleware injects the per-engine on-disk directory roots
// (uploads, streams, logs) the handlers consult at request time. The
// fields are pre-resolved with their documented defaults by
// ResolvePathDefaults, so handlers always see a non-empty value.
// Threading them via context — instead of a process-global — is what
// lets tests pass per-test tempdirs and run in parallel without
// colliding on filesystem paths.
func pathDirsMiddleware(uploadDir, streamDir, logsDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		handlers.SetUploadDirOnContext(c, uploadDir)
		handlers.SetStreamDirOnContext(c, streamDir)
		handlers.SetLogsDirOnContext(c, logsDir)
		c.Next()
	}
}

// ---------------------------------------------------------------------------
// Templates and static assets
// ---------------------------------------------------------------------------

func parseTemplates(assets fs.FS, store *config.Store) (*template.Template, error) {
	funcMap := buildFuncMap(store)
	return template.New("").Funcs(funcMap).ParseFS(assets, "web/templates/**/*.html")
}

func buildFuncMap(store *config.Store) template.FuncMap {
	// configuredLocation returns the Store's configured *time.Location,
	// falling back to nil when none is set or the value doesn't parse.
	// Closing over the per-engine Store keeps the helper deterministic
	// in tests that construct their own engine and timezone.
	configuredLocation := func() *time.Location {
		tz := store.Timezone()
		if tz == "" {
			return nil
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil
		}
		return loc
	}

	return template.FuncMap{
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
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
			if loc := configuredLocation(); loc != nil {
				return t.In(loc).Format(utils.LayoutDateTimeLocal)
			}
			return t.Local().Format(utils.LayoutDateTimeLocal)
		},
		"formatDate": func(t time.Time) string {
			if loc := configuredLocation(); loc != nil {
				return t.In(loc).Format(utils.LayoutDate)
			}
			return t.Format(utils.LayoutDate)
		},
		"formatDateTime": func(t time.Time) string {
			if loc := configuredLocation(); loc != nil {
				return t.In(loc).Format(utils.LayoutDateTime)
			}
			return t.Format(utils.LayoutDateTime)
		},
		"isZeroDate": func(t time.Time) bool {
			return utils.IsZeroDate(t)
		},
		"div": func(a, b int) int { return a / b },
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
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
			if loc := configuredLocation(); loc != nil {
				return time.Now().In(loc)
			}
			return time.Now()
		},
		"markdownify": func(t string) template.HTML {
			unsafe := blackfriday.Run([]byte(t))
			safe := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
			return template.HTML(safe)
		},
		"csrfToken": func() string {
			// Placeholder — overridden per-request by middleware via c.Set.
			return ""
		},
		"linebreaks": func(s string) template.HTML {
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
}

// registerStaticRoutes wires up /static/, /fonts/, /uploads, and /favicon.ico.
// Uploads remain on the live filesystem (./uploads); everything else comes
// from cfg.Assets.
func registerStaticRoutes(r *gin.Engine, assets fs.FS) {
	r.Static("/uploads", "./uploads")

	r.GET("/static/*filepath", func(c *gin.Context) {
		filePath := fmt.Sprintf("web/static%s", c.Param("filepath"))
		data, err := fs.ReadFile(assets, filePath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		http.ServeContent(c.Writer, c.Request, filePath, time.Time{}, strings.NewReader(string(data)))
	})

	r.GET("/fonts/*filepath", func(c *gin.Context) {
		filePath := fmt.Sprintf("utils/fonts%s", c.Param("filepath"))
		data, err := fs.ReadFile(assets, filePath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		http.ServeContent(c.Writer, c.Request, filePath, time.Time{}, strings.NewReader(string(data)))
	})

	r.GET("/favicon.ico", func(c *gin.Context) {
		data, err := fs.ReadFile(assets, "web/static/img/favicon.ico")
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to load favicon")
			return
		}
		c.Data(http.StatusOK, "image/x-icon", data)
	})
}
