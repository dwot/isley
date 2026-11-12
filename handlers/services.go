package handlers

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"isley/logger"
	"isley/service"
)

const contextKeyServices = "services"

// ServicesFromContext returns the engine's service layer. Handlers that
// have moved onto the service layer call it instead of writing SQL.
func ServicesFromContext(c *gin.Context) *service.Services {
	return c.MustGet(contextKeyServices).(*service.Services)
}

// SetServicesOnContext binds svc to a request; the engine middleware uses it.
func SetServicesOnContext(c *gin.Context, svc *service.Services) {
	c.Set(contextKeyServices, svc)
}

// legacyKeys maps service error codes to the translation keys the web UI
// has always shown for them.
var legacyKeys = map[string]string{
	service.CodeMetricBuiltIn:            "api_metric_cannot_delete",
	service.CodeActivityTypeNameReserved: "api_activity_name_reserved",
	service.CodeLastStatusEntry:          "api_cannot_delete_last_status",
	service.CodeStatusEntryNotFound:      "api_status_not_found",
	service.CodePlantNotFound:            "api_plant_not_found",
	service.CodeImageNotFound:            "api_image_not_found",
	service.CodeStrainNotFound:           "api_strain_not_found",
	service.CodeStrainInUse:              "api_strain_in_use",
	service.CodeBreederNotFound:          "api_breeder_not_found",
	service.CodeBreederNameTaken:         "api_breeder_name_taken",
	service.CodeLineageEntryNotFound:     "api_lineage_not_found",
	service.CodeLineageCycle:             "api_lineage_cycle",
	service.CodeSensorNotFound:           "api_sensor_not_found",
	service.CodeStreamNotFound:           "api_stream_not_found",
	service.CodeSmokeReportNotFound:      "api_smoke_report_not_found",
	service.CodeSmokeCriterionNotFound:   "api_smoke_criterion_not_found",
}

// apiServiceError answers a legacy (non-v1) endpoint with the
// {"error": "..."} shape the web UI expects. fallbackKey is the translation
// key for an internal error.
func apiServiceError(c *gin.Context, err error, fallbackKey string) {
	e, ok := service.AsError(err)
	if !ok {
		logger.Log.WithError(err).WithField("path", c.FullPath()).Error("Request failed")
		apiInternalError(c, fallbackKey)
		return
	}
	message := e.Message
	if key, ok := legacyKeys[e.Code]; ok {
		message = key
	} else if len(e.Fields) > 0 {
		names := make([]string, 0, len(e.Fields))
		for name := range e.Fields {
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, len(names))
		for i, name := range names {
			parts[i] = name + " " + e.Fields[name]
		}
		message = strings.Join(parts, "; ")
	}
	status := http.StatusBadRequest
	if e.Kind == service.KindNotFound {
		status = http.StatusNotFound
	}
	apiError(c, status, message)
}
