package routes

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"isley/config"
	"isley/handlers"
	"isley/utils"
	"net/http"
)

func AddBasicRoutes(r *gin.RouterGroup, version string) {
	r.GET("/", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/index.html", gin.H{
			"title":           "Dashboard",
			"currentPath":     currentPath,
			"version":         version,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
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
		c.HTML(http.StatusOK, "views/plants.html", gin.H{
			"title":           "Plants",
			"currentPath":     currentPath,
			"version":         version,
			"zones":           config.Zones,
			"strains":         config.Strains,
			"statuses":        config.Statuses,
			"measurements":    config.Metrics,
			"breeders":        config.Breeders,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

	r.GET("/strains", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/strains.html", gin.H{
			"title":           "Strains",
			"currentPath":     currentPath,
			"version":         version,
			"strains":         config.Strains,
			"breeders":        config.Breeders,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

	r.GET("/graph/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/graph.html", gin.H{
			"title":           "Sensor Graphs",
			"currentPath":     currentPath,
			"version":         version,
			"SensorID":        c.Param("id"),
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
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
		c.HTML(http.StatusOK, "views/plant-add.html", gin.H{
			"title":           "Add Plant",
			"currentPath":     currentPath,
			"version":         version,
			"zones":           config.Zones,
			"strains":         config.Strains,
			"statuses":        config.Statuses,
			"breeders":        config.Breeders,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

	r.GET("/plant/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/plant.html", gin.H{
			"title":           "Plant Details",
			"currentPath":     currentPath,
			"version":         version,
			"plant":           handlers.GetPlant(c.Param("id")),
			"zones":           config.Zones,
			"strains":         config.Strains,
			"statuses":        config.Statuses,
			"breeders":        config.Breeders,
			"measurements":    config.Metrics,
			"sensors":         handlers.GetSensors(),
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
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
		c.HTML(http.StatusOK, "views/strain-add.html", gin.H{
			"title":           "Add Strain",
			"currentPath":     currentPath,
			"version":         version,
			"prefillName":     c.Query("name"),
			"breeders":        config.Breeders,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

	r.GET("/strain/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/strain.html", gin.H{
			"title":           "Strain Details",
			"currentPath":     currentPath,
			"version":         version,
			"strain":          handlers.GetStrain(c.Param("id")),
			"breeders":        config.Breeders,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
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
		groupedSensors := handlers.GetGroupedSensorsWithLatestReading()
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
	r.POST("/record-multi-activity", handlers.RecordMultiPlantActivity)
	r.POST("/settings", handlers.SaveSettings)
}

// AddExternalApiRoutes External API endpoints
func AddExternalApiRoutes(r *gin.RouterGroup) {
	r.POST("/api/sensors/ingest", handlers.RateLimitMiddleware(handlers.IngestRateLimiter), handlers.IngestSensorData)
	r.GET("/api/overlay", handlers.GetOverlayData)
}

func AddProtectedRoutes(r *gin.RouterGroup, version string) {
	r.GET("/plant/:id/edit", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/plant-edit.html", gin.H{
			"title":           "Edit Plant",
			"currentPath":     currentPath,
			"version":         version,
			"plant":           handlers.GetPlant(c.Param("id")),
			"zones":           config.Zones,
			"strains":         config.Strains,
			"statuses":        config.Statuses,
			"breeders":        config.Breeders,
			"measurements":    config.Metrics,
			"sensors":         handlers.GetSensors(),
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

	r.GET("/strain/:id/edit", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/strain-edit.html", gin.H{
			"title":           "Edit Strain",
			"currentPath":     currentPath,
			"version":         version,
			"strain":          handlers.GetStrain(c.Param("id")),
			"breeders":        config.Breeders,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

	r.GET("/settings", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/settings.html", gin.H{
			"title":           "Settings",
			"currentPath":     currentPath,
			"version":         version,
			"settings":        handlers.GetSettings(),
			"zones":           config.Zones,
			"metrics":         config.Metrics,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"breeders":        config.Breeders,
			"streams":         handlers.GetStreams(),
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

	r.GET("/sensors", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		currentPath, _ := c.Get("currentPath")
		c.HTML(http.StatusOK, "views/sensors.html", gin.H{
			"title":           "Sensors",
			"currentPath":     currentPath,
			"version":         version,
			"settings":        handlers.GetSettings(),
			"sensors":         handlers.GetSensors(),
			"zones":           config.Zones,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
			"csrfToken":       c.GetString("csrf_token"),
		})
	})

}
