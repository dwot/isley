package apiv1

import (
	_ "embed"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/scope"
	"isley/service"
)

// Prefix is where the API is mounted.
const Prefix = "/api/v1"

//go:embed openapi.yaml
var spec []byte

//go:embed guide.html
var guide []byte

// DocsPath is the guide page; SpecPath is the OpenAPI document. Both are
// public: they describe the API and hold no data.
const (
	DocsPath = Prefix + "/docs"
	SpecPath = Prefix + "/openapi.yaml"
)

// Spec returns the OpenAPI document served at /api/v1/openapi.yaml.
func Spec() []byte { return spec }

type api struct {
	svc *service.Services
}

// Register mounts /api/v1 on r.
func Register(r *gin.Engine, svc *service.Services, verify KeyVerifier) {
	a := &api{svc: svc}

	r.GET(SpecPath, func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", spec)
	})
	r.GET(DocsPath, func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", guide)
	})

	g := r.Group(Prefix)
	g.Use(requireKey(verify), a.idempotent)

	read := g.Group("")
	read.Use(requireScope(scope.Read))
	read.GET("/instance", a.getInstance)
	read.GET("/statuses", a.listStatuses)
	read.GET("/metrics", a.listMetrics)
	read.GET("/activity-types", a.listActivityTypes)
	read.GET("/zones", a.listZones)
	read.GET("/zones/:id", a.getZone)
	read.GET("/plants", a.listPlants)
	read.GET("/plants/:id", a.getPlant)
	read.GET("/plants/:id/status-history", a.listStatusHistory)
	read.GET("/plants/:id/activities", a.listPlantActivities)
	read.GET("/plants/:id/measurements", a.listMeasurements)
	read.GET("/plants/:id/images", a.listPlantImages)
	read.GET("/images/:id", a.getImage)
	read.GET("/breeders", a.listBreeders)
	read.GET("/breeders/:id", a.getBreeder)
	read.GET("/strains", a.listStrains)
	read.GET("/strains/:id", a.getStrain)
	read.GET("/strains/:id/lineage", a.getLineage)
	read.GET("/strains/:id/descendants", a.listDescendants)
	read.GET("/sensors", a.listSensors)
	read.GET("/sensors/:id", a.getSensor)
	read.GET("/sensors/:id/data", a.getSensorData)
	read.GET("/streams", a.listStreams)
	read.GET("/streams/:id", a.getStream)
	read.GET("/smoke-criteria", a.listSmokeCriteria)
	read.GET("/plants/:id/smoke-reports", a.listPlantSmokeReports)
	read.GET("/smoke-reports/:id", a.getSmokeReport)
	read.GET("/strains/:id/smoke-summary", a.getStrainSmokeSummary)
	read.GET("/activities", a.listActivities)
	read.GET("/activities/:id", a.getActivity)

	write := g.Group("")
	write.Use(requireScope(scope.Write))
	write.POST("/plants", a.createPlant)
	write.PATCH("/plants/:id", a.updatePlant)
	write.PUT("/plants/:id/sensors", a.setPlantSensors)
	write.POST("/plants/:id/status", a.changePlantStatus)
	write.PATCH("/plants/:id/status-history/:entry_id", a.updateStatusEntry)
	write.DELETE("/plants/:id/status-history/:entry_id", a.deleteStatusEntry)
	write.POST("/plants/:id/activities", a.createActivity)
	write.POST("/activities/bulk", a.createActivitiesBulk)
	write.PATCH("/activities/:id", a.updateActivity)
	write.DELETE("/activities/:id", a.deleteActivity)
	write.POST("/plants/:id/measurements", a.createMeasurement)
	write.PATCH("/measurements/:id", a.updateMeasurement)
	write.DELETE("/measurements/:id", a.deleteMeasurement)
	write.POST("/plants/:id/images", a.createImage)
	write.DELETE("/images/:id", a.deleteImage)
	write.POST("/breeders", a.createBreeder)
	write.PATCH("/breeders/:id", a.updateBreeder)
	write.POST("/strains", a.createStrain)
	write.PATCH("/strains/:id", a.updateStrain)
	write.POST("/strains/:id/seed-count", a.adjustSeedCount)
	write.PUT("/strains/:id/lineage", a.setLineage)
	write.POST("/plants/:id/smoke-reports", a.createSmokeReport)
	write.PATCH("/smoke-reports/:id", a.updateSmokeReport)
	write.DELETE("/smoke-reports/:id", a.deleteSmokeReport)

	admin := g.Group("")
	admin.Use(requireScope(scope.Admin))
	admin.POST("/zones", a.createZone)
	admin.PATCH("/zones/:id", a.updateZone)
	admin.DELETE("/zones/:id", a.deleteZone)
	admin.DELETE("/plants/:id", a.deletePlant)
	admin.DELETE("/breeders/:id", a.deleteBreeder)
	admin.DELETE("/strains/:id", a.deleteStrain)
	admin.PATCH("/sensors/:id", a.updateSensor)
	admin.DELETE("/sensors/:id", a.deleteSensor)
	admin.POST("/streams", a.createStream)
	admin.PATCH("/streams/:id", a.updateStream)
	admin.DELETE("/streams/:id", a.deleteStream)
}

func itoa(n int) string { return strconv.Itoa(n) }

// IsAPIPath reports whether path belongs to /api/v1.
func IsAPIPath(path string) bool {
	return path == Prefix || strings.HasPrefix(path, Prefix+"/")
}

// NotFound answers an unknown /api/v1 path with the error envelope. The
// engine calls it from its NoRoute handler.
func NotFound(c *gin.Context) {
	abortWith(c, http.StatusNotFound, CodeNotFound, "no such endpoint: "+c.Request.Method+" "+c.Request.URL.Path, nil)
}
