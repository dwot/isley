package routes

import (
	"github.com/gin-gonic/gin"
	"isley/handlers"
	"net/http"
)

func AddProtectedRotues(r *gin.RouterGroup, version string) {

	plants, _ := handlers.GetLivingPlants()
	activities := handlers.GetActivities()

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/index.html", gin.H{
			"title":        "Dashboard",
			"version":      version,
			"sensorLatest": handlers.GetSensorLatest(),
			"plants":       plants,
			"activities":   activities,
		})
	})

	r.GET("/plants", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/plants.html", gin.H{
			"title":      "Plants",
			"version":    version,
			"plantList":  handlers.GetPlantList(),
			"zones":      handlers.GetZones(),
			"strains":    handlers.GetStrains(),
			"statuses":   handlers.GetStatuses(),
			"breeders":   handlers.GetBreederList(),
			"plants":     plants,
			"activities": activities,
		})
	})

	r.GET("/strains", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/strains.html", gin.H{
			"title":      "Strains",
			"version":    version,
			"strains":    handlers.GetStrains(),
			"breeders":   handlers.GetBreederList(),
			"plants":     plants,
			"activities": activities,
		})
	})

	r.GET("/graph/:id", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/graph.html", gin.H{
			"title":      "Sensor Graphs",
			"version":    version,
			"SensorID":   c.Param("id"),
			"SensorName": handlers.GetSensorName(c.Param("id")),
			"plants":     plants,
			"activities": activities,
		})
	})

	r.GET("/plant/:id", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/plant.html", gin.H{
			"title":        "Plant Details",
			"version":      version,
			"plant":        handlers.GetPlant(c.Param("id")),
			"zones":        handlers.GetZones(),
			"strains":      handlers.GetStrains(),
			"statuses":     handlers.GetStatuses(),
			"breeders":     handlers.GetBreederList(),
			"measurements": handlers.GetMeasurements(),
			"sensors":      handlers.GetSensors(),
			"plants":       plants,
			"activities":   activities,
		})
	})

	r.GET("/settings", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/settings.html", gin.H{
			"title":      "Settings",
			"version":    version,
			"settings":   handlers.GetSettings(),
			"zones":      handlers.GetZones(),
			"metrics":    handlers.GetMeasurements(),
			"plants":     plants,
			"activities": activities,
		})
	})

	r.GET("/sensors", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/sensors.html", gin.H{
			"title":      "Sensors",
			"version":    version,
			"settings":   handlers.GetSettings(),
			"sensors":    handlers.GetSensors(),
			"zones":      handlers.GetZones(),
			"plants":     plants,
			"activities": activities,
		})
	})

	r.GET("/graphs", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/graphs.html", gin.H{
			"title":      "Graphs",
			"version":    version,
			"sensors":    handlers.GetGroupedSensors(),
			"plants":     plants,
			"activities": activities,
		})
	})

	// API endpoints
	r.POST("/plants", handlers.AddPlant)
	r.GET("/plants/living", handlers.LivingPlantsHandler)
	r.GET("/plants/harvested", handlers.HarvestedPlantsHandler)
	r.GET("/plants/dead", handlers.DeadPlantsHandler)

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
	r.POST("/decorateImage", handlers.DecorateImageHandler)

	r.POST("/sensors/scanACI", handlers.ScanACInfinitySensors)
	r.POST("/sensors/scanEC", handlers.ScanEcoWittSensors)
	r.POST("/sensors/edit", handlers.EditSensor)
	r.DELETE("/sensors/delete/:id", handlers.DeleteSensor)
	r.GET("/sensorData", handlers.ChartHandler)
	r.GET("/sensors/grouped", func(c *gin.Context) {
		groupedSensors := handlers.GetGroupedSensorsWithLatestReading()
		c.JSON(http.StatusOK, groupedSensors)
	})

	r.POST("/strains", handlers.AddStrainHandler)
	r.GET("/strains/:id", handlers.GetStrainHandler)
	r.PUT("/strains/:id", handlers.UpdateStrainHandler)
	r.DELETE("/strains/:id", handlers.DeleteStrainHandler)
	r.GET("/strains/in-stock", handlers.InStockStrainsHandler)
	r.GET("/strains/out-of-stock", handlers.OutOfStockStrainsHandler)

	r.POST("/settings", handlers.SaveSettings)
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

	r.POST("/settings/upload-logo", handlers.UploadLogo)
	r.POST("/record-multi-activity", handlers.RecordMultiPlantActivity)

}
