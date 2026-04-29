package routes

import (
	"isley/handlers"
	"isley/model"
	"isley/utils"
	"net/http"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func AddBasicRoutes(r *gin.RouterGroup, version string) {
	r.GET("/", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/index.html", gin.H{
			"title":           "Dashboard",
			"currentPath":     currentPath,
			"version":         version,
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
			"pollInterval":    store.PollingInterval(),
		})
	})

	// Translations API - returns JSON map of translations for the requested language
	r.GET("/api/translations", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.JSON(http.StatusOK, translations)
	})

	r.GET("/plants", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/plants.html", gin.H{
			"title":           "Plants",
			"currentPath":     currentPath,
			"version":         version,
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"statuses":        store.Statuses(),
			"measurements":    store.Metrics(),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/activities", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")

		db := handlers.DBFromContext(c)
		filters, err := handlers.ParseActivityLogFilters(c)
		if err != nil {
			// Bad filter input — render the page with empty filters but flag
			// the error to the user via a query param check in the template.
			filters = handlers.ActivityLogFilters{Order: "date_desc"}
		}

		page := 1
		if v := c.Query("page"); v != "" {
			if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
				page = n
			}
		}

		pageCtx, err := handlers.BuildActivityLogPageContext(db, filters, page, c.Request.URL.Query())
		if err != nil {
			pageCtx = handlers.ActivityLogPageContext{}
		}

		store := handlers.ConfigStoreFromContext(c)
		activities := store.Activities()
		c.HTML(http.StatusOK, "views/activities.html", gin.H{
			"title":           "Activity Log",
			"currentPath":     currentPath,
			"version":         version,
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"breeders":        store.Breeders(),
			"activityTypes":   activities,
			"activityCtx":     pageCtx,
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})
	r.GET("/activities/list", handlers.ListAllActivities)

	r.GET("/strains", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/strains.html", gin.H{
			"title":           "Strains",
			"currentPath":     currentPath,
			"version":         version,
			"strains":         store.Strains(),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/graph/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/graph.html", gin.H{
			"title":           "Sensor Graphs",
			"currentPath":     currentPath,
			"version":         version,
			"SensorID":        c.Param("id"),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/plant/new", func(c *gin.Context) {
		// Redirect to login if not authenticated
		if sessions.Default(c).Get("logged_in") == nil {
			c.Redirect(http.StatusFound, "/login")
			return
		}
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/plant-add.html", gin.H{
			"title":           "Add Plant",
			"currentPath":     currentPath,
			"version":         version,
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"statuses":        store.Statuses(),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/plant/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		db := handlers.DBFromContext(c)
		idStr := c.Param("id")
		plant := handlers.GetPlant(db, idStr)
		prevPlantID, nextPlantID := handlers.GetAdjacentPlantIDs(db, int(plant.ID))
		c.HTML(http.StatusOK, "views/plant.html", gin.H{
			"title":           "Plant Details",
			"currentPath":     currentPath,
			"version":         version,
			"plant":           plant,
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"statuses":        store.Statuses(),
			"breeders":        store.Breeders(),
			"measurements":    store.Metrics(),
			"sensors":         handlers.GetSensors(db),
			"plants":          handlers.GetLivingPlants(db),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
			"prevPlantID":     prevPlantID,
			"nextPlantID":     nextPlantID,
		})
	})

	r.GET("/strain/new", func(c *gin.Context) {
		// Redirect to login if not authenticated
		if sessions.Default(c).Get("logged_in") == nil {
			c.Redirect(http.StatusFound, "/login")
			return
		}
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/strain-add.html", gin.H{
			"title":           "Add Strain",
			"currentPath":     currentPath,
			"version":         version,
			"prefillName":     c.Query("name"),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/strain/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/strain.html", gin.H{
			"title":           "Strain Details",
			"currentPath":     currentPath,
			"version":         version,
			"strain":          handlers.GetStrain(handlers.DBFromContext(c), c.Param("id")),
			"breeders":        store.Breeders(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/listFonts", utils.ListFontsHandler)
	r.GET("/listLogos", utils.ListLogosHandler)

	r.GET("/plants/living", handlers.LivingPlantsHandler)
	r.GET("/plants/harvested", handlers.HarvestedPlantsHandler)
	r.GET("/plants/dead", handlers.DeadPlantsHandler)
	r.GET("/plants/by-strain/:strainID", handlers.PlantsByStrainHandler)
	r.GET("/sensorData", handlers.ChartHandler)
	r.GET("/sensors/grouped", func(c *gin.Context) {
		groupedSensors := handlers.GetGroupedSensorsWithLatestReading(
			handlers.DBFromContext(c), handlers.SensorCacheServiceFromContext(c))
		c.JSON(http.StatusOK, groupedSensors)
	})
	r.GET("/strains/:id", handlers.GetStrainHandler)
	r.GET("/strains/in-stock", handlers.InStockStrainsHandler)
	r.GET("/strains/out-of-stock", handlers.OutOfStockStrainsHandler)
	r.POST("/decorateImage", utils.DecorateImageHandler)
	r.GET("/streams", handlers.GetStreamsByZoneHandler)

	// Lineage (public read)
	r.GET("/strains/:id/lineage", handlers.GetLineageHandler)
	r.GET("/strains/:id/descendants", handlers.GetDescendantsHandler)
	r.GET("/strains/lookup", handlers.LookupStrainByName)
}

func AddProtectedApiRoutes(r *gin.RouterGroup) {
	// API endpoints
	r.POST("/plants", handlers.AddPlant)
	r.POST("/plant", func(c *gin.Context) { handlers.UpdatePlant(c) })
	r.POST("/plant/status", handlers.UpdatePlantStatus)
	r.DELETE("/plant/delete/:id", handlers.DeletePlant)
	r.POST("/plant/link-sensors", handlers.LinkSensorsToPlant)
	r.POST("/plantStatus/edit", handlers.EditStatus)
	r.DELETE("/plantStatus/delete/:id", handlers.DeleteStatus)
	r.POST("/plantMeasurement", func(c *gin.Context) { handlers.CreatePlantMeasurement(c) })
	r.POST("/plantMeasurement/edit", handlers.EditMeasurement)
	r.DELETE("/plantMeasurement/delete/:id", handlers.DeleteMeasurement)
	r.POST("/plantActivity", func(c *gin.Context) { handlers.CreatePlantActivity(c) })
	r.POST("/plantActivity/edit", handlers.EditActivity)
	r.DELETE("/plantActivity/delete/:id", handlers.DeleteActivity)
	r.POST("/plant/:plantID/images/upload", handlers.UploadPlantImages)
	r.DELETE("/plant/images/:imageID/delete", handlers.DeletePlantImage)

	r.POST("/sensors/scanACI", handlers.ScanACInfinitySensors)
	r.POST("/sensors/scanEC", handlers.ScanEcoWittSensors)
	r.POST("/sensors/edit", handlers.EditSensor)
	r.DELETE("/sensors/delete/:id", handlers.DeleteSensor)

	// Debug endpoint to dump raw AC Infinity API response
	r.GET("/sensors/dumpACI", handlers.DumpACInfinityJSON)

	r.POST("/strains", handlers.AddStrainHandler)

	r.PUT("/strains/:id", handlers.UpdateStrainHandler)
	r.DELETE("/strains/:id", handlers.DeleteStrainHandler)

	// Lineage (protected write)
	r.POST("/strains/:id/lineage", handlers.AddLineageHandler)
	r.PUT("/strains/:id/lineage", handlers.SetLineageHandler)
	r.PUT("/lineage/:lineageID", handlers.UpdateLineageHandler)
	r.DELETE("/lineage/:lineageID", handlers.DeleteLineageHandler)

	r.POST("/aci/login", handlers.ACILoginHandler)

	r.POST("/zones", handlers.AddZoneHandler)
	r.PUT("/zones/:id", handlers.UpdateZoneHandler)
	r.DELETE("/zones/:id", handlers.DeleteZoneHandler)

	r.POST("/metrics", handlers.AddMetricHandler)
	r.GET("/metrics", handlers.GetMetricsHandler)
	r.PUT("/metrics/:id", handlers.UpdateMetricHandler)
	r.DELETE("/metrics/:id", handlers.DeleteMetricHandler)

	r.POST("/activities", handlers.AddActivityHandler)
	r.PUT("/activities/:id", handlers.UpdateActivityHandler)
	r.DELETE("/activities/:id", handlers.DeleteActivityHandler)
	r.POST("/streams", handlers.AddStreamHandler)
	r.PUT("/streams/:id", handlers.UpdateStreamHandler)
	r.DELETE("/streams/:id", handlers.DeleteStreamHandler)

	r.POST("/breeders", handlers.AddBreederHandler)
	r.PUT("/breeders/:id", handlers.UpdateBreederHandler)
	r.DELETE("/breeders/:id", handlers.DeleteBreederHandler)

	r.POST("/settings/upload-logo", handlers.UploadLogo)
	r.GET("/settings/logs", handlers.GetLogs)
	r.GET("/settings/logs/download", handlers.DownloadLogs)
	r.POST("/settings/backup/create", handlers.CreateBackup)
	r.GET("/settings/backup/status", handlers.GetBackupStatus)
	r.GET("/settings/backup/list", handlers.ListBackups)
	r.GET("/settings/backup/download/:name", handlers.DownloadBackup)
	r.DELETE("/settings/backup/:name", handlers.DeleteBackup)
	r.POST("/settings/backup/restore", handlers.ImportBackup)
	r.GET("/settings/backup/restore/status", handlers.GetRestoreStatus)
	r.GET("/settings/backup/sqlite/download", handlers.DownloadSQLiteDB)
	r.POST("/settings/backup/sqlite/upload", handlers.UploadSQLiteDB)
	r.POST("/record-multi-activity", handlers.RecordMultiPlantActivity)
	r.POST("/settings", handlers.SaveSettings)
}

// AddExternalApiRoutes External API endpoints
func AddExternalApiRoutes(r *gin.RouterGroup) {
	r.POST("/api/sensors/ingest", handlers.IngestRateLimitMiddleware(), handlers.IngestSensorData)
	r.GET("/api/overlay", handlers.IngestRateLimitMiddleware(), handlers.GetOverlayData)
}

func AddProtectedRoutes(r *gin.RouterGroup, version string) {
	r.GET("/plant/:id/edit", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/plant-edit.html", gin.H{
			"title":           "Edit Plant",
			"currentPath":     currentPath,
			"version":         version,
			"plant":           handlers.GetPlant(handlers.DBFromContext(c), c.Param("id")),
			"zones":           store.Zones(),
			"strains":         store.Strains(),
			"statuses":        store.Statuses(),
			"breeders":        store.Breeders(),
			"measurements":    store.Metrics(),
			"sensors":         handlers.GetSensors(handlers.DBFromContext(c)),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/strain/:id/edit", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/strain-edit.html", gin.H{
			"title":           "Edit Strain",
			"currentPath":     currentPath,
			"version":         version,
			"strain":          handlers.GetStrain(handlers.DBFromContext(c), c.Param("id")),
			"breeders":        store.Breeders(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/settings", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/settings.html", gin.H{
			"title":           "Settings",
			"currentPath":     currentPath,
			"version":         version,
			"settings":        handlers.GetSettings(handlers.DBFromContext(c)),
			"zones":           store.Zones(),
			"metrics":         store.Metrics(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"breeders":        store.Breeders(),
			"streams":         handlers.GetStreams(handlers.DBFromContext(c)),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"dbDriver":        model.GetDriver(),
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	r.GET("/sensors", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		store := handlers.ConfigStoreFromContext(c)
		c.HTML(http.StatusOK, "views/sensors.html", gin.H{
			"title":           "Sensors",
			"currentPath":     currentPath,
			"version":         version,
			"settings":        handlers.GetSettings(handlers.DBFromContext(c)),
			"sensors":         handlers.GetSensors(handlers.DBFromContext(c)),
			"zones":           store.Zones(),
			"plants":          handlers.GetLivingPlants(handlers.DBFromContext(c)),
			"activities":      store.Activities(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
			"cspNonce":        c.GetString("cspNonce"),
		})
	})

	// Activity log exports (auth-gated; unauthenticated users are redirected
	// to /login by AuthMiddleware).  Filters mirror the /activities page.
	r.GET("/activities/export/csv", handlers.ExportActivitiesCSV)
	r.GET("/activities/export/xlsx", handlers.ExportActivitiesXLSX)
}
