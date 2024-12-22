package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
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

//go:embed model/migrations/*.sql web/templates/* web/static/**/*  utils/fonts/* VERSION
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
		"json": func(v interface{}) string {
			a, err := json.Marshal(v)
			if err != nil {
				logger.Log.WithError(err).Error("Error marshalling JSON")
				return ""
			}
			return string(a)
		},
		"formatDate": func(t time.Time) string {
			return t.Format("01/02/2006")
		},
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
	}

	// Attach FuncMap and ParseFS
	templ := template.Must(template.New("").Funcs(funcMap).ParseFS(embeddedFiles, "web/templates/**/*"))

	// Set HTML templates in Gin
	r.SetHTMLTemplate(templ)

	// Load settings (PollingInterval, ACIEnabled, etc.)
	loadSettings()

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
	r.Use(sessions.Sessions("isley_session", store))

	// Public routes
	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/login.html", gin.H{})
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

	protected := r.Group("/")
	protected.Use(AuthMiddleware())
	{
		protected.Use(ForcePasswordChangeMiddleware())

		protected.GET("/change-password", func(c *gin.Context) {
			c.HTML(http.StatusOK, "views/change-password.html", gin.H{})
		})

		protected.POST("/change-password", func(c *gin.Context) {
			handleChangePassword(c)
		})

		routes.AddProtectedRotues(protected, version)
	}

	// Start the server
	logger.Log.Fatal(r.Run(":" + port))
	logger.Log.Info("Server started on port %s", port)
}

// Helper functions
func loadSettings() {
	strPollingInterval, err := handlers.GetSetting("polling_interval")
	if err == nil {
		if iPollingInterval, err := strconv.Atoi(strPollingInterval); err == nil {
			config.PollingInterval = iPollingInterval
		}
	}

	strACIEnabled, err := handlers.GetSetting("aci.enabled")
	if err == nil {
		if iACIEnabled, err := strconv.Atoi(strACIEnabled); err == nil {
			config.ACIEnabled = iACIEnabled
		}
	}

	strECEnabled, err := handlers.GetSetting("ec.enabled")
	if err == nil {
		if iECEnabled, err := strconv.Atoi(strECEnabled); err == nil {
			config.ECEnabled = iECEnabled
		}
	}

	strACIToken, err := handlers.GetSetting("aci.token")
	if err == nil {
		config.ACIToken = strACIToken
	}

	strECDevices, err := handlers.LoadEcDevices()
	if err == nil {
		config.ECDevices = strECDevices
	}

	config.Activities = handlers.GetActivities()
	config.Metrics = handlers.GetMetrics()
	config.Statuses = handlers.GetStatuses()
	config.Zones = handlers.GetZones()
	config.Strains = handlers.GetStrains()
	config.Breeders = handlers.GetBreeders()
}

func handleLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	storedUsername, _ := handlers.GetSetting("auth_username")
	storedPasswordHash, _ := handlers.GetSetting("auth_password")
	forcePasswordChange, _ := handlers.GetSetting("force_password_change")

	if username != storedUsername || !utils.CheckPasswordHash(password, storedPasswordHash) {
		c.HTML(http.StatusUnauthorized, "views/login.html", gin.H{
			"Error": "Invalid username or password",
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
		c.HTML(http.StatusBadRequest, "views/change-password.html", gin.H{
			"Error": "Passwords do not match",
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
