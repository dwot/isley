package routes

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"isley/handlers"
	"isley/utils"
)

// page renders a full page. It fills what every page's layout needs (the
// header and language menu, the multi-plant activity modal, CSRF and CSP
// tokens); fields adds the page's own data and may override any of it.
func page(c *gin.Context, version, template, title string, fields gin.H) {
	lang := utils.GetLanguage(c)
	currentPath, _ := c.Get("currentPath")
	data := gin.H{
		"title":           title,
		"currentPath":     currentPath,
		"version":         version,
		"plants":          handlers.GetLivingPlants(c.Request.Context(), handlers.ServicesFromContext(c)),
		"activities":      handlers.ConfigStoreFromContext(c).Activities(),
		"loggedIn":        sessions.Default(c).Get("logged_in"),
		"lcl":             utils.TranslationService.GetTranslations(lang),
		"languages":       utils.AvailableLanguages,
		"currentLanguage": lang,
		"csrfToken":       c.GetString("csrf_token"),
		"cspNonce":        c.GetString("cspNonce"),
	}
	for k, v := range fields {
		data[k] = v
	}
	c.HTML(http.StatusOK, template, data)
}

// requireLogin sends a visitor who isn't logged in to /login. Pages that
// add things live in the basic routes (readable in guest mode) but only
// make sense to a logged-in user.
func requireLogin(c *gin.Context) bool {
	if sessions.Default(c).Get("logged_in") == nil {
		c.Redirect(http.StatusFound, "/login")
		return false
	}
	return true
}
