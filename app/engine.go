package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"github.com/sirupsen/logrus"

	"isley/apiv1"
	"isley/chrono"
	"isley/config"
	"isley/handlers"
	"isley/logger"
	"isley/mcpserver"
	"isley/model"
	"isley/ratelimit"
	"isley/service"
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
	r, _, err := NewEngineAndServices(cfg)
	return r, err
}

// NewEngineAndServices is NewEngine that also returns the service layer
// the engine was built with, for background work (the sensor watcher)
// that should share its caches and clock.
func NewEngineAndServices(cfg Config) (*gin.Engine, *service.Services, error) {
	if cfg.DB == nil {
		return nil, nil, fmt.Errorf("app.NewEngine: cfg.DB is required")
	}
	if cfg.Assets == nil {
		return nil, nil, fmt.Errorf("app.NewEngine: cfg.Assets is required")
	}
	if len(cfg.SessionSecret) < 32 {
		return nil, nil, fmt.Errorf("app.NewEngine: cfg.SessionSecret must be at least 32 bytes (got %d)", len(cfg.SessionSecret))
	}

	cfg = ResolvePathDefaults(cfg)

	configStore := cfg.ConfigStore
	if configStore == nil {
		configStore = config.NewStore()
	}

	r := gin.New()

	if len(cfg.TrustedProxies) > 0 {
		if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			return nil, nil, fmt.Errorf("set trusted proxies: %w", err)
		}
	} else {
		// Explicitly trust no one when empty — Gin's default trusts all
		// proxies which is unsafe.
		_ = r.SetTrustedProxies(nil)
	}

	r.Use(gin.Recovery())
	r.Use(gin.LoggerWithWriter(logger.AccessWriter))
	r.Use(securityHeadersMiddleware(configStore, hstsHeaderValue(cfg)))
	r.Use(currentPathMiddleware())

	templ, err := parseTemplates(cfg.Assets, configStore, cfg.Clock)
	if err != nil {
		return nil, nil, fmt.Errorf("parse templates: %w", err)
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
	backupSvc.SetVersion(releaseVersion(cfg))
	r.Use(backupServiceMiddleware(backupSvc))

	rateLimiterSvc := cfg.RateLimiterService
	if rateLimiterSvc == nil {
		rateLimiterSvc = ratelimit.NewService(nil, nil)
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

	services, err := newServices(cfg, configStore, sensorCacheSvc)
	if err != nil {
		return nil, nil, err
	}
	r.Use(func(c *gin.Context) {
		handlers.SetServicesOnContext(c, services)
		c.Next()
	})

	registerPublicRoutes(r, cfg)
	registerProtectedRoutes(r, cfg)
	registerAPIRoutes(r)

	apiv1.Register(r, services, apiKeyVerifier(cfg.DB))
	r.Any(mcpserver.Path, gin.WrapH(mcpserver.Handler(services, mcpKeyVerifier(cfg.DB), mcpserver.Options{
		Version: releaseVersion(cfg),
		Enabled: func() bool { return configStore.MCPEnabled() == 1 },
	})))

	r.NoRoute(func(c *gin.Context) {
		if apiv1.IsAPIPath(c.Request.URL.Path) {
			apiv1.NotFound(c)
			return
		}
		c.String(http.StatusNotFound, "404 page not found")
	})

	return r, services, nil
}

// newServices builds the service layer for this engine: the dialect, the
// clock, and the time-zone boundary (instance zone from the timezone
// setting, system zone detected from the database).
func newServices(cfg Config, store *config.Store, sensorCache *handlers.SensorCacheService) (*service.Services, error) {
	dialect := cfg.Dialect
	if dialect == "" {
		d, err := service.ParseDialect(model.GetDriver())
		if err != nil {
			return nil, fmt.Errorf("app.NewEngine: set cfg.Dialect: %w", err)
		}
		dialect = d
	}
	clock := cfg.Clock
	if clock == nil {
		clock = chrono.System()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	systemZone := cfg.ServerTimeZone
	if systemZone == nil {
		var err error
		if systemZone, err = chrono.SystemZone(ctx, cfg.DB, dialect.IsPostgres()); err != nil {
			return nil, fmt.Errorf("app.NewEngine: %w", err)
		}
	}
	// Stored times move to UTC once (#141 Phase 2). Wall times were written
	// in the instance zone, which is also how they have been read, so the
	// pages and the API show the same times after as before.
	wallZone := chrono.NewZones(store.Timezone, nil).Instance()
	// No deadline: on a large database the conversion takes as long as it
	// takes, once.
	convStart := time.Now()
	converted, err := service.EnsureUTCStorage(context.Background(), cfg.DB, dialect, wallZone, systemZone)
	if err != nil {
		return nil, fmt.Errorf("app.NewEngine: %w", err)
	}
	if converted {
		logger.Log.WithField("took", time.Since(convStart).String()).Info("UTC conversion finished")
	}

	return service.New(service.Deps{
		Version: releaseVersion(cfg),
		DB:      cfg.DB,
		Dialect: dialect,
		Clock:   clock,
		Zones:   chrono.NewUTCZones(store.Timezone),
		Store:   store,
		// Resolved by NewEngine before this runs.
		UploadDir: cfg.UploadDir,
		StreamDir: cfg.StreamDir,
		SensorsChanged: func() {
			sensorCache.GroupedReset()
			sensorCache.DataReset()
		},
	})
}

// apiKeyVerifier adapts the API key check to /api/v1.
func apiKeyVerifier(db *sql.DB) apiv1.KeyVerifier {
	return func(_ context.Context, plaintext string) (*apiv1.Key, error) {
		key, err := handlers.VerifyAPIKey(db, plaintext)
		if err != nil || key == nil {
			return nil, err
		}
		return &apiv1.Key{ID: key.ID, Name: key.Name, Scopes: key.Scopes}, nil
	}
}

// releaseVersion is the version Isley reports to clients.
func releaseVersion(cfg Config) string {
	if cfg.ReleaseVersion != "" {
		return cfg.ReleaseVersion
	}
	return cfg.Version
}

// mcpKeyVerifier adapts the API key check to /mcp.
func mcpKeyVerifier(db *sql.DB) mcpserver.KeyVerifier {
	return func(_ context.Context, plaintext string) (*mcpserver.Key, error) {
		key, err := handlers.VerifyAPIKey(db, plaintext)
		if err != nil || key == nil {
			return nil, err
		}
		return &mcpserver.Key{ID: key.ID, Name: key.Name, Scopes: key.Scopes}, nil
	}
}

// keyOnlyPath reports whether path is served by API key alone, never a
// browser session: /api/v1 and /mcp.
func keyOnlyPath(path string) bool {
	return apiv1.IsAPIPath(path) || mcpserver.IsPath(path)
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// hstsHeaderValue assembles the Strict-Transport-Security header value
// from the engine Config, or returns "" when HSTS is disabled.
func hstsHeaderValue(cfg Config) string {
	if cfg.HSTSMaxAge <= 0 {
		return ""
	}
	v := fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge)
	if cfg.HSTSIncludeSubdomains {
		v += "; includeSubDomains"
		if cfg.HSTSPreload {
			v += "; preload"
		}
	}
	return v
}

func securityHeadersMiddleware(store *config.Store, hsts string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		if hsts != "" {
			c.Header("Strict-Transport-Security", hsts)
		}

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
		connectSrc := "'self'"
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
				"script-src 'self' 'unsafe-eval' 'nonce-"+nonce+"'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"font-src 'self' data:; "+
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

// csrfMiddleware issues a per-session token and validates it on
// POST/PUT/PATCH/DELETE. Pure API-key clients (no browser session) are exempt — they
// carry no token and can't be driven cross-site. A logged-in session must
// always present its token, even when an X-API-KEY header is also set, so the
// session-gated endpoints can't bypass CSRF by attaching a key header.
//
// /api/v1 and /mcp are skipped entirely: they authenticate by API key only
// and never look at the session, so there is nothing for a cross-site
// request to ride on, and API responses shouldn't set session cookies.
func csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if keyOnlyPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		session := sessions.Default(c)

		token, _ := session.Get("csrf_token").(string)
		if token == "" {
			token = handlers.GenerateCSRFToken()
			session.Set("csrf_token", token)
			_ = session.Save()
		}
		c.Set("csrf_token", token)

		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" || c.Request.Method == "DELETE" {
			loggedIn, _ := session.Get("logged_in").(bool)
			if !loggedIn && c.GetHeader("X-API-KEY") != "" {
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
// *ratelimit.Service into the Gin context. The ingest
// middleware and the /login handler resolve the active limiter from
// context, which is what lets every test construct its own service
// (and therefore call t.Parallel() without colliding on counter state).
func rateLimiterServiceMiddleware(svc *ratelimit.Service) gin.HandlerFunc {
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

func parseTemplates(assets fs.FS, store *config.Store, clock chrono.Clock) (*template.Template, error) {
	funcMap := buildFuncMap(store, clock)
	funcMap["asset"] = assetURL(assets)
	return template.New("").Funcs(funcMap).ParseFS(assets, "web/templates/**/*.html")
}

// assetURL returns the "asset" template function: a /static/ path with a
// short hash of the file's content, "/static/js/x.js?v=1a2b3c4d". A
// changed file gets a new URL, so browsers never run a stale copy after an
// upgrade, and a versioned URL can be cached for good. Hashes are computed
// once per path; the bundle can't change while the process runs.
func assetURL(assets fs.FS) func(string) string {
	var hashes sync.Map // path -> versioned URL
	return func(path string) string {
		if v, ok := hashes.Load(path); ok {
			return v.(string)
		}
		data, err := fs.ReadFile(assets, "web"+path)
		if err != nil {
			logger.Log.WithField("path", path).Warn("asset: no such static file")
			return path
		}
		sum := sha256.Sum256(data)
		url := fmt.Sprintf("%s?v=%x", path, sum[:4])
		hashes.Store(path, url)
		return url
	}
}

func buildFuncMap(store *config.Store, clock chrono.Clock) template.FuncMap {
	if clock == nil {
		clock = chrono.System()
	}
	// Templates show times in the instance zone, decided by the same
	// boundary the services use. Only the instance side is needed here.
	zones := chrono.NewZones(store.Timezone, nil)
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
			return t.In(zones.Instance()).Format(utils.LayoutDateTimeLocal)
		},
		"formatDate": func(t time.Time) string {
			return t.In(zones.Instance()).Format(utils.LayoutDate)
		},
		"formatDateTime": func(t time.Time) string {
			return t.In(zones.Instance()).Format(utils.LayoutDateTime)
		},
		"isZeroDate": func(t time.Time) bool {
			return utils.IsZeroDate(t)
		},
		// weightValue renders a gram value (the storage unit) in the unit a
		// page chose to display, e.g. 56.7 -> "2" for ounces.
		"weightValue": func(grams float64, unit string) string {
			return utils.FormatWeight(grams, unit)
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
		"now": func() time.Time { return zones.Now(clock) },
		// instanceZone names the zone pages show and read times in, for
		// isley-time.js.
		"instanceZone": func() string { return zones.Instance().String() },
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
	}
}

// staticCacheMaxAge is how long browsers and CDNs may reuse an embedded
// asset before revalidating. Asset URLs are not fingerprinted, so this is
// kept to an hour: after a deploy the old file is served for at most that
// long, and the ETag turns the revalidation into a 304 rather than a
// re-download.
const staticCacheMaxAge = time.Hour

// assetStartTime is used as Last-Modified for embedded assets. The embed FS
// carries no modtimes, and a process (re)start is the only time the bundle
// can change.
var assetStartTime = time.Now().UTC().Truncate(time.Second)

// serveEmbeddedAsset returns a handler that serves files under root in the
// embedded FS with the headers browsers and Cloudflare need to cache them:
// Cache-Control, a content-hash ETag, and Last-Modified. Without these every
// page navigation refetched every script and stylesheet through the proxy.
// ETags are computed once per path and memoised; the bundle is immutable
// for the life of the process.
func serveEmbeddedAsset(assets fs.FS, root string) gin.HandlerFunc {
	var etags sync.Map // path -> etag string
	return func(c *gin.Context) {
		filePath := root + c.Param("filepath")
		data, err := fs.ReadFile(assets, filePath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		etag, ok := etags.Load(filePath)
		if !ok {
			sum := sha256.Sum256(data)
			etag = fmt.Sprintf(`"%x"`, sum[:16])
			etags.Store(filePath, etag)
		}
		h := c.Writer.Header()
		h.Set("ETag", etag.(string))
		if c.Query("v") != "" {
			// A versioned URL from the asset template function: its
			// content never changes.
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(staticCacheMaxAge.Seconds())))
		}
		// ServeContent handles Range, If-None-Match and If-Modified-Since,
		// and sets Content-Type from the extension and Last-Modified from
		// the modtime.
		http.ServeContent(c.Writer, c.Request, filePath, assetStartTime, strings.NewReader(string(data)))
	}
}

// registerStaticRoutes wires up /static/, /fonts/, /uploads, and /favicon.ico.
// Uploads remain on the live filesystem (./uploads); everything else comes
// from cfg.Assets.
func registerStaticRoutes(r *gin.Engine, assets fs.FS) {
	r.Static("/uploads", "./uploads")

	r.GET("/static/*filepath", serveEmbeddedAsset(assets, "web/static"))
	r.GET("/fonts/*filepath", serveEmbeddedAsset(assets, "utils/fonts"))

	r.GET("/favicon.ico", func(c *gin.Context) {
		data, err := fs.ReadFile(assets, "web/static/img/favicon.ico")
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to load favicon")
			return
		}
		c.Data(http.StatusOK, "image/x-icon", data)
	})
}
