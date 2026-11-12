package routes

import (
	"isley/cannadb"
	"isley/handlers"
	"isley/model"
	"isley/scope"
	"isley/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AddBasicRoutes(r *gin.RouterGroup, version string) {
	r.GET("/", func(c *gin.Context) {
		page(c, version, "views/index.html", "Dashboard", gin.H{
			"pollInterval": handlers.ConfigStoreFromContext(c).PollingInterval(),
		})
	})

	// Translations API - returns JSON map of translations for the requested language
	r.GET("/api/translations", func(c *gin.Context) {
		lang := utils.GetLanguage(c)
		translations := utils.TranslationService.GetTranslations(lang)
		c.JSON(http.StatusOK, translations)
	})

	r.GET("/plants", func(c *gin.Context) {
		store := handlers.ConfigStoreFromContext(c)
		page(c, version, "views/plants.html", "Plants", gin.H{
			"zones": store.Zones(), "strains": store.Strains(), "statuses": store.Statuses(),
			"measurements": store.Metrics(), "breeders": store.Breeders(),
		})
	})

	r.GET("/activities", func(c *gin.Context) {
		store := handlers.ConfigStoreFromContext(c)
		page(c, version, "views/activities.html", "Activity Log", gin.H{
			"zones": store.Zones(), "strains": store.Strains(), "breeders": store.Breeders(),
			"activityTypes": store.Activities(),
		})
	})
	r.GET("/activities/list", handlers.ListAllActivities)

	r.GET("/strains", func(c *gin.Context) {
		store := handlers.ConfigStoreFromContext(c)
		page(c, version, "views/strains.html", "Strains", gin.H{
			"strains": store.Strains(), "breeders": store.Breeders(),
			"cannadbEnabled": store.CannadbEnabled() == 1,
		})
	})

	r.GET("/graph/:id", func(c *gin.Context) {
		page(c, version, "views/graph.html", "Sensor Graphs", gin.H{"SensorID": c.Param("id")})
	})

	r.GET("/plant/new", func(c *gin.Context) {
		if !requireLogin(c) {
			return
		}
		store := handlers.ConfigStoreFromContext(c)
		page(c, version, "views/plant-add.html", "Add Plant", gin.H{
			"zones": store.Zones(), "strains": store.Strains(), "statuses": store.Statuses(),
			"breeders": store.Breeders(),
		})
	})

	r.GET("/plant/:id", func(c *gin.Context) {
		store := handlers.ConfigStoreFromContext(c)
		svc := handlers.ServicesFromContext(c)
		plant := handlers.GetPlant(c.Request.Context(), svc, c.Param("id"))
		prevPlantID, nextPlantID := handlers.GetAdjacentPlantIDs(c.Request.Context(), svc, int(plant.ID))
		// smokeHarvested gates the "+ Add" trigger alone: a new report may
		// only ever be created on a currently-harvested plant. smokeEligible
		// (harvested || has reports) gates the card and its modals, so a
		// plant that leaves its harvested statuses (e.g. a mis-logged Curing
		// entry gets deleted from the status-history modal) does not orphan
		// reports it already has — see handlers.SmokeEligibility.
		smokeHarvested, smokeHasReports := handlers.SmokeEligibility(c.Request.Context(), svc, int(plant.ID))
		page(c, version, "views/plant.html", "Plant Details", gin.H{
			"plant": plant, "zones": store.Zones(), "strains": store.Strains(), "statuses": store.Statuses(),
			"breeders": store.Breeders(), "measurements": store.Metrics(),
			"sensors":     handlers.GetSensors(c.Request.Context(), svc),
			"prevPlantID": prevPlantID, "nextPlantID": nextPlantID,
			"smokeCriteria":    store.SmokeCriteria(),
			"smokeRatingScale": handlers.SmokeRatingScale(),
			"smokeEligible":    smokeHarvested || smokeHasReports,
			"smokeHarvested":   smokeHarvested,
		})
	})

	r.GET("/strain/new", func(c *gin.Context) {
		if !requireLogin(c) {
			return
		}
		page(c, version, "views/strain-add.html", "Add Strain", gin.H{
			"prefillName": c.Query("name"), "breeders": handlers.ConfigStoreFromContext(c).Breeders(),
		})
	})

	r.GET("/strain/:id", func(c *gin.Context) {
		strain := handlers.GetStrain(c.Request.Context(), handlers.ServicesFromContext(c), c.Param("id"))
		page(c, version, "views/strain.html", "Strain Details", gin.H{
			"strain": strain, "cannadbURL": cannadb.WebURL(strain.CannadbURI),
			"breeders": handlers.ConfigStoreFromContext(c).Breeders(),
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
			c.Request.Context(), handlers.ServicesFromContext(c), handlers.SensorCacheServiceFromContext(c))
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
	r.GET("/strains/:id/smoke-summary", handlers.GetStrainSmokeSummaryHandler)

	// Smoke reports (read visibility follows the plant page: public under
	// guest mode, behind AuthMiddleware otherwise — see AddProtectedApiRoutes
	// below for the write side, and app/routes.go for which group this
	// basic-routes block lands in per mode).
	r.GET("/plant/:id/smoke-reports", handlers.GetSmokeReportsHandler)
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

	// CannaDB import (search + one-click import). Auth-gated so anonymous
	// users can't drive outbound calls against the shared per-IP rate budget.
	r.GET("/strains/cannadb/search", handlers.CannadbSearchHandler)
	r.POST("/strains/cannadb/import", handlers.CannadbImportHandler)

	// Lineage (protected write)
	r.POST("/strains/:id/lineage", handlers.AddLineageHandler)
	r.PUT("/strains/:id/lineage", handlers.SetLineageHandler)
	r.PUT("/lineage/:lineageID", handlers.UpdateLineageHandler)
	r.DELETE("/lineage/:lineageID", handlers.DeleteLineageHandler)

	r.POST("/aci/login", handlers.ACILoginHandler)

	r.POST("/zones", handlers.AddZoneHandler)
	r.PUT("/zones/:id", handlers.UpdateZoneHandler)
	r.DELETE("/zones/:id", handlers.DeleteZoneHandler)

	r.POST("/statuses/:id/vpd", handlers.UpdateStatusVPDHandler)

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

	// Smoke report criteria (Settings › Customization). Session-gated like the
	// rest of the settings CRUD.
	r.GET("/smoke-criteria", handlers.GetSmokeCriteriaHandler)
	r.POST("/smoke-criteria", handlers.AddSmokeCriterionHandler)
	r.PUT("/smoke-criteria/:id", handlers.UpdateSmokeCriterionHandler)
	r.DELETE("/smoke-criteria/:id", handlers.DeleteSmokeCriterionHandler)

	// Smoke reports (protected write). The POST uses :plantID because the
	// existing POST tree on /plant/ already binds that name; the GET above
	// uses :id for the same reason. Gin panics on a mismatch within a method.
	r.POST("/plant/:plantID/smoke-reports", handlers.AddSmokeReportHandler)
	r.PUT("/smoke-reports/:id", handlers.UpdateSmokeReportHandler)
	r.DELETE("/smoke-reports/:id", handlers.DeleteSmokeReportHandler)
}

// AddExternalApiRoutes External API endpoints
func AddExternalApiRoutes(r *gin.RouterGroup) {
	r.POST("/api/sensors/ingest", handlers.RequireKeyScope(scope.Ingest), handlers.IngestRateLimitMiddleware(), handlers.IngestSensorData)
	r.GET("/api/overlay", handlers.RequireKeyScope(scope.Read), handlers.IngestRateLimitMiddleware(), handlers.GetOverlayData)
}

func AddProtectedRoutes(r *gin.RouterGroup, version string) {
	r.GET("/plant/:id/edit", func(c *gin.Context) {
		store := handlers.ConfigStoreFromContext(c)
		svc := handlers.ServicesFromContext(c)
		page(c, version, "views/plant-edit.html", "Edit Plant", gin.H{
			"plant": handlers.GetPlant(c.Request.Context(), svc, c.Param("id")),
			"zones": store.Zones(), "strains": store.Strains(), "statuses": store.Statuses(),
			"breeders": store.Breeders(), "measurements": store.Metrics(),
			"sensors": handlers.GetSensors(c.Request.Context(), svc),
		})
	})

	r.GET("/strain/:id/edit", func(c *gin.Context) {
		page(c, version, "views/strain-edit.html", "Edit Strain", gin.H{
			"strain":   handlers.GetStrain(c.Request.Context(), handlers.ServicesFromContext(c), c.Param("id")),
			"breeders": handlers.ConfigStoreFromContext(c).Breeders(),
		})
	})

	r.GET("/settings", func(c *gin.Context) {
		store := handlers.ConfigStoreFromContext(c)
		svc := handlers.ServicesFromContext(c)
		page(c, version, "views/settings.html", "Settings", gin.H{
			"settings": handlers.GetSettings(handlers.DBFromContext(c)),
			"zones":    store.Zones(), "statuses": store.Statuses(), "metrics": store.Metrics(),
			"breeders": store.Breeders(), "smokeCriteria": store.SmokeCriteria(),
			"sensors":  handlers.GetSensors(c.Request.Context(), svc),
			"streams":  handlers.GetStreams(c.Request.Context(), svc),
			"dbDriver": model.GetDriver(),
		})
	})

	// API key management. Session-only (never X-API-KEY) so a leaked key can't
	// mint or revoke other keys.
	r.GET("/settings/api-keys", handlers.GetAPIKeysHandler)
	r.POST("/settings/api-keys", handlers.CreateAPIKeyHandler)
	r.POST("/settings/api-keys/:id/regenerate", handlers.RegenerateAPIKeyHandler)
	r.PUT("/settings/api-keys/:id/scopes", handlers.UpdateAPIKeyScopesHandler)
	r.DELETE("/settings/api-keys/:id", handlers.RevokeAPIKeyHandler)

	r.GET("/sensors", func(c *gin.Context) {
		page(c, version, "views/sensors.html", "Sensors", gin.H{
			"settings": handlers.GetSettings(handlers.DBFromContext(c)),
			"sensors":  handlers.GetSensors(c.Request.Context(), handlers.ServicesFromContext(c)),
			"zones":    handlers.ConfigStoreFromContext(c).Zones(),
		})
	})

	// Activity log exports (auth-gated; unauthenticated users are redirected
	// to /login by AuthMiddleware).  Filters mirror the /activities page.
	r.GET("/activities/export/csv", handlers.ExportActivitiesCSV)
	r.GET("/activities/export/xlsx", handlers.ExportActivitiesXLSX)
}
