package main

import (
	"encoding/json"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"html/template"
	"isley/config"
	"isley/handlers"
	"isley/model"
	"isley/routes"
	"isley/utils"
	"isley/watcher"
	"log"
	"net/http"
	"strconv"
	"time"
)

func main() {
	model.MigrateDB()

	// Start the sensor watcher
	go watcher.Watch()

	// Set up Gin router
	r := gin.Default()

	// Add the `json` function to the template functions map
	r.SetFuncMap(template.FuncMap{
		"json": func(v interface{}) string {
			a, err := json.Marshal(v)
			if err != nil {
				log.Printf("error marshalling JSON: %v", err)
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
				log.Printf("error parsing date: %v", err)
				return t
			}
			return tm.Format("01/02/2006")
		},
		"toInt": func(value interface{}) int {
			switch v := value.(type) {
			case string:
				intVal, _ := strconv.Atoi(v)
				return intVal
			case float64:
				return int(v)
			case int:
				return v
			default:
				return 0
			}
		},
		"preview": func(t string) string {
			if len(t) > 100 {
				return t[:100] + "..."
			}
			return t
		},
	})

	// Set title base to Isley v0.0.1a
	version := "Isley 0.0.1a"

	//Set PollingInterval
	strPollingInterval, err := handlers.GetSetting("polling_interval")
	if err == nil {
		iPollingInterval, err := strconv.Atoi(strPollingInterval)
		if err == nil {
			config.PollingInterval = iPollingInterval
		}
	}
	//Set ACIEnabled
	strACIEnabled, err := handlers.GetSetting("aci.enabled")
	if err == nil {
		iACIEnabled, err := strconv.Atoi(strACIEnabled)
		if err == nil {
			config.ACIEnabled = iACIEnabled
		}
	}
	//Set ECEnabled
	strECEnabled, err := handlers.GetSetting("ec.enabled")
	if err == nil {
		iECEnabled, err := strconv.Atoi(strECEnabled)
		if err == nil {
			config.ECEnabled = iECEnabled
		}
	}
	//Set ACIToken
	strACIToken, err := handlers.GetSetting("aci.token")
	if err == nil {
		config.ACIToken = strACIToken
	}
	//Set ECDevices
	strECDevices, err := handlers.LoadEcDevices()
	if err == nil {
		config.ECDevices = strECDevices
	}
	//Set Activities
	config.Activities = handlers.GetActivities()
	//Set Metrics
	config.Metrics = handlers.GetMetrics()
	//Set Statuses
	config.Statuses = handlers.GetStatuses()
	//Set Zones
	config.Zones = handlers.GetZones()
	//Set Strains
	config.Strains = handlers.GetStrains()
	//Set Breeders
	config.Breeders = handlers.GetBreeders()

	// Serve static files
	r.Static("/static", "./web/static")
	r.Static("/uploads", "./uploads")
	r.StaticFile("/favicon.ico", "./web/static/img/favicon.ico")

	// Serve HTML templates
	r.LoadHTMLGlob("./web/templates/**/*")

	// Initialize session store
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("isley_session", store))

	// Initialize default admin credentials if not present
	if _, err := handlers.GetSetting("auth_username"); err != nil {
		handlers.UpdateSetting("auth_username", "admin")
		hashedPassword, _ := utils.HashPassword("isley")
		handlers.UpdateSetting("auth_password", hashedPassword)
		handlers.UpdateSetting("force_password_change", "true")
	}

	// Public routes
	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/login.html", gin.H{})
	})

	r.POST("/login", func(c *gin.Context) {
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

		// Successful login
		session := sessions.Default(c)
		session.Set("logged_in", true)
		session.Set("force_password_change", forcePasswordChange == "true")
		session.Save()

		if forcePasswordChange == "true" {
			c.Redirect(http.StatusFound, "/change-password")
			return
		}

		c.Redirect(http.StatusFound, "/")
	})

	r.GET("/logout", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Clear()
		session.Save()
		c.Redirect(http.StatusFound, "/login")
	})

	protected := r.Group("/")
	protected.Use(AuthMiddleware())
	{
		protected.Use(ForcePasswordChangeMiddleware())

		protected.GET("/change-password", func(c *gin.Context) {
			c.HTML(http.StatusOK, "views/change-password.html", gin.H{})
		})

		protected.POST("/change-password", func(c *gin.Context) {
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
		})

		routes.AddProtectedRotues(protected, version)
	}

	// Start the server
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
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
