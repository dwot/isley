package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"isley/handlers"
	"isley/logger"
	"isley/routes"
	"isley/utils"
)

// registerPublicRoutes wires the routes that have no auth requirement:
// /login, /logout, /health, and the dashboard suite when guest mode is on.
func registerPublicRoutes(r *gin.Engine, cfg Config) {
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
		if !handlers.RateLimiterServiceFromContext(c).Login().Allow(c.ClientIP()) {
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		handlers.HandleLogin(c)
	})

	r.GET("/logout", handlers.HandleLogout)
	r.GET("/health", handleHealth)

	if cfg.GuestMode {
		routes.AddBasicRoutes(r.Group("/"), cfg.Version)
	}
}

// registerProtectedRoutes wires the auth-gated route groups. When guest
// mode is off, the basic routes (dashboard, plant pages) live here too.
func registerProtectedRoutes(r *gin.Engine, cfg Config) {
	protected := r.Group("/")
	protected.Use(handlers.AuthMiddleware())
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

	protected.POST("/change-password", handlers.HandleChangePassword)

	routes.AddProtectedRoutes(protected, cfg.Version)

	if !cfg.GuestMode {
		routes.AddBasicRoutes(protected, cfg.Version)
	}
}

// registerAPIRoutes wires the AuthMiddlewareApi-gated routes (browser
// session OR X-API-KEY).
func registerAPIRoutes(r *gin.Engine) {
	apiProtected := r.Group("/")
	apiProtected.Use(handlers.AuthMiddlewareApi())
	routes.AddProtectedApiRoutes(apiProtected)
	routes.AddExternalApiRoutes(apiProtected)
}

// handleHealth answers the Dockerfile HEALTHCHECK and any external probe.
// It pings the database with a short timeout so an offline DB surfaces as
// 503, rather than the container being reported healthy with no backend.
func handleHealth(c *gin.Context) {
	db := handlers.DBFromContext(c)
	if db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": "db not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		logger.Log.WithError(err).Warn("Health check: database ping failed")
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "error": "db unreachable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
