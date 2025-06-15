package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
	"github.com/sirupsen/logrus"
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
	"time"
)

//go:embed model/migrations/sqlite/*.sql model/migrations/postgres/*.sql web/templates/**/*.html web/static/**/* utils/fonts/* VERSION
var embeddedFiles embed.FS

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

	// Start the sensor watcher
	watcher.PruneSensorData()
	go watcher.Watch()

	// Set up Gin router
	r := gin.Default()

	funcMap := template.FuncMap{
		"upper": strings.ToUpper, // Define the 'upper' function
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
		"formatStringDateTimeLocal": func(t string) string {
			parsedTime, err := time.Parse(time.RFC3339, t)
			if err != nil {
				return "" // Return empty if parsing fails
			}
			return parsedTime.Format("2006-01-02T15:04")
		},
		"formatDateTimeLocal": func(t time.Time) string {
			return t.Format("2006-01-02T15:04")
		},
		"toLocalTimeString": func(t time.Time) string {
			if err != nil {
				return "" // Fallback to the original string if parsing fails
			}
			return t.In(time.Local).Format("01/02/2006 03:04 PM")
		},
		"formatDate": func(t time.Time) string {
			return t.Format("01/02/2006")
		},
		"formatDateTime": func(t time.Time) string { return t.Format("01/02/2006 03:04 PM") },
		"formatDateISO": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		"formatStringDate": func(t string) string {
			tm, err := time.Parse(time.RFC3339, t)
			if err != nil {
				logger.Log.WithFields(logrus.Fields{
					"input": t,
					"error": err,
				}).Error("Error parsing date")
				return t
			}
			return tm.Format("01/02/2006")
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
		"linebreaks": func(s string) template.HTML {
			// Replace newlines with <br> tags and mark as safe HTML
			return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
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
		http.ServeContent(c.Writer, c.Request, filePath, time.Now().In(time.Local), strings.NewReader(string(data)))
	})

	r.GET("/fonts/*filepath", func(c *gin.Context) {
		filePath := fmt.Sprintf("utils/fonts%s", c.Param("filepath"))
		data, err := embeddedFiles.ReadFile(filePath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		http.ServeContent(c.Writer, c.Request, filePath, time.Now().In(time.Local), strings.NewReader(string(data)))
	})

	// Initialize session store
	store := cookie.NewStore([]byte("secret"))
	store.Options(sessions.Options{
		Path: "/",
	})
	r.Use(sessions.Sessions("isley_session", store))

	// Public routes
	r.GET("/login", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusOK, "views/login.html", gin.H{
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
		})
	})

	r.POST("/login", func(c *gin.Context) {
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
			c.HTML(http.StatusOK, "views/change-password.html", gin.H{
				"lcl":             translations,
				"languages":       utils.AvailableLanguages,
				"currentLanguage": lang,
			})
		})

		protected.POST("/change-password", func(c *gin.Context) {
			handleChangePassword(c)
		})

		routes.AddProtectedRotues(protected, version)

		if !guestMode {
			routes.AddBasicRoutes(protected, version)
		}
	}

	apiProtected := r.Group("/")
	apiProtected.Use(AuthMiddlewareApi())
	{
		routes.AddProtectedApiRoutes(apiProtected)
	}

	// Start the server
	logger.Log.Fatal(r.Run(":" + port))
	logger.Log.Info("Server started on port", port)
}

func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
	c.String(http.StatusOK, "Isley is running")
	logger.Log.Info("Health check passed")
}

func handleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	storedUsername, _ := handlers.GetSetting("auth_username")
	storedPasswordHash, _ := handlers.GetSetting("auth_password")
	forcePasswordChange, _ := handlers.GetSetting("force_password_change")

	if username != storedUsername || !utils.CheckPasswordHash(password, storedPasswordHash) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusUnauthorized, "views/login.html", gin.H{
			"Error":           "Invalid username or password",
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
		})
		return
	}

	session := sessions.Default(c)
	session.Set("logged_in", true)
	session.Set("force_password_change", forcePasswordChange == "true")
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

func handleChangePassword(c *gin.Context) {
	newPassword := c.PostForm("new_password")
	confirmPassword := c.PostForm("confirm_password")

	if newPassword != confirmPassword {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusBadRequest, "views/change-password.html", gin.H{
			"Error":           "Passwords do not match",
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
		})
		return
	}

	hashedPassword, _ := utils.HashPassword(newPassword)

	handlers.UpdateSetting("auth_password", hashedPassword)
	handlers.UpdateSetting("force_password_change", "false")

	session := sessions.Default(c)
	session.Set("force_password_change", false)
	session.Save()

	c.Redirect(http.StatusFound, "/")
}

func AuthMiddlewareApi() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-KEY")
		session := sessions.Default(c)
		loggedIn := session.Get("logged_in")

		if apiKey != "" {
			// Get API key from settings
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
				// Log error
				logger.Log.WithError(err).Error("Error retrieving API key from database")

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Could not validate API key",
				})
				return
			}

			if apiKey != storedAPIKey {
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
