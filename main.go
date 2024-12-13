package main

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"html/template"
	"isley/handlers"
	"isley/model"
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
		//Add formatDate for format "2006-01-02"
		"formatDateISO": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		"formatStringDate": func(t string) string {
			//input is "2024-12-11T04:58:52Z"
			//output is "12/11/2024"
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

	// Serve static files from the "./static" directory
	r.Static("/static", "./web/static")
	r.Static("/css", "./web/static/css")
	r.Static("/img", "./web/static/img")
	r.Static("/scss", "./web/static/scss")
	r.Static("/vendor", "./web/static/vendor")
	r.Static("/js", "./web/static/js")
	r.Static("/uploads", "./uploads")
	r.StaticFile("/favicon.ico", "./web/static/img/favicon.ico")

	// Serve HTML templates
	r.LoadHTMLGlob("./web/templates/**/*")

	// Pages
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/index.html", gin.H{
			"title":        "Dashboard",
			"version":      version,
			"plantList":    handlers.GetPlantList(),
			"sensorLatest": handlers.GetSensorLatest(),
		})
	})

	r.GET("/plants", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/plants.html", gin.H{
			"title":     "Plants",
			"version":   version,
			"plantList": handlers.GetPlantList(),
			"zones":     handlers.GetZones(),
			"strains":   handlers.GetStrains(),
			"statuses":  handlers.GetStatuses(),
			"breeders":  handlers.GetBreederList(),
		})
	})

	r.GET("/strains", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/strains.html", gin.H{
			"title":    "Strains",
			"version":  version,
			"strains":  handlers.GetStrains(),
			"breeders": handlers.GetBreederList(),
		})
	})

	r.GET("/graph/:id", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/graph.html", gin.H{
			"title":    "Sensor Graphs",
			"version":  version,
			"SensorID": c.Param("id"),
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
			"activities":   handlers.GetActivities(),
			"sensors":      handlers.GetSensors(),
		})
	})

	r.GET("/settings", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/settings.html", gin.H{
			"title":      "Settings",
			"version":    version,
			"settings":   handlers.GetSettings(),
			"zones":      handlers.GetZones(),
			"metrics":    handlers.GetMeasurements(),
			"activities": handlers.GetActivities(),
		})
	})

	r.GET("/sensors", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/sensors.html", gin.H{
			"title":    "Sensors",
			"version":  version,
			"settings": handlers.GetSettings(),
			"sensors":  handlers.GetSensors(),
			"zones":    handlers.GetZones(),
		})
	})

	r.GET("/graphs", func(c *gin.Context) {
		c.HTML(http.StatusOK, "views/graphs.html", gin.H{
			"title":   "Graphs",
			"version": version,
			"sensors": handlers.GetGroupedSensors(),
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

	r.POST("/sensors/scanACI", handlers.ScanACInfinitySensors)
	r.POST("/sensors/scanEC", handlers.ScanEcoWittSensors)
	r.POST("/sensors/edit", handlers.EditSensor)
	r.DELETE("/sensors/delete/:id", handlers.DeleteSensor)
	r.GET("/sensorData/:id/:duration", func(c *gin.Context) { handlers.ChartHandler(c, c.Param("id"), c.Param("duration")) })
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

	// Start server
	err := r.Run(":8080")
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
