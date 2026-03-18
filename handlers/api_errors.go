package handlers

import (
	"isley/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

// T translates a message key for the current request's language.
// Falls back to the English translation if the target locale is missing the key,
// and falls back to the key itself if no translation exists at all.
func T(c *gin.Context, key string) string {
	lang := utils.GetLanguage(c)
	translations := utils.TranslationService.GetTranslations(lang)
	if msg, ok := translations[key]; ok && msg != "" {
		return msg
	}
	return key // fallback: return the key as-is
}

// apiError sends a standardized JSON error response.
// The message parameter should be a translation key (e.g. "api_database_error").
// It is automatically translated for the request's language.
func apiError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": T(c, message)})
}

// apiOK sends a standardized JSON success response with a translated message.
func apiOK(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{"message": T(c, message)})
}

// Common error helpers for the most frequent status codes.

func apiBadRequest(c *gin.Context, message string) {
	apiError(c, http.StatusBadRequest, message)
}

func apiInternalError(c *gin.Context, message string) {
	apiError(c, http.StatusInternalServerError, message)
}

func apiNotFound(c *gin.Context, message string) {
	apiError(c, http.StatusNotFound, message)
}

func apiForbidden(c *gin.Context, message string) {
	apiError(c, http.StatusForbidden, message)
}
