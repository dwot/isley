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
		c.HTML(http.StatusOK, "views/index.html", gin.H{
			"title":           "Dashboard",
			"version":         version,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
		})
	})

	r.GET("/plants", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusOK, "views/plants.html", gin.H{
			"title":           "Plants",
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
		})
	})

	r.GET("/strains", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusOK, "views/strains.html", gin.H{
			"title":           "Strains",
			"version":         version,
			"strains":         config.Strains,
			"breeders":        config.Breeders,
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
		})
	})

	r.GET("/graph/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusOK, "views/graph.html", gin.H{
			"title":           "Sensor Graphs",
			"version":         version,
			"SensorID":        c.Param("id"),
			"plants":          handlers.GetLivingPlants(),
			"activities":      config.Activities,
			"loggedIn":        sessions.Default(c).Get("logged_in"),
			"lcl":             translations,
			"languages":       utils.AvailableLanguages,
			"currentLanguage": lang,
		})
	})

	r.GET("/plant/:id", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusOK, "views/plant.html", gin.H{
			"title":           "Plant Details",
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
}

func AddProtectedApiRoutes(r *gin.RouterGroup) {
	// API endpoints
	r.POST("/plants", handlers.AddPlant)
	r.POST("/plant", func(c *gin.Context) { handlers.UpdatePlant(c) })
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

	r.POST("/strains", handlers.AddStrainHandler)

	r.PUT("/strains/:id", handlers.UpdateStrainHandler)
	r.DELETE("/strains/:id", handlers.DeleteStrainHandler)

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
	r.POST("/record-multi-activity", handlers.RecordMultiPlantActivity)
	r.POST("/settings", handlers.SaveSettings)
}

func AddProtectedRotues(r *gin.RouterGroup, version string) {
	r.GET("/settings", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusOK, "views/settings.html", gin.H{
			"title":           "Settings",
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
		})
	})

	r.GET("/sensors", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.HTML(http.StatusOK, "views/sensors.html", gin.H{
			"title":           "Sensors",
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
		})
	})

}
