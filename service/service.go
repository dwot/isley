package service

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"isley/chrono"
	"isley/config"
)

// Deps is what every service needs. app.NewEngine builds one per engine.
type Deps struct {
	// Version is the running Isley version, reported by Instance.
	Version string
	DB      *sql.DB
	Dialect Dialect
	Clock   chrono.Clock
	Zones   *chrono.Zones
	Store   *config.Store
	// UploadDir is where uploaded files live (images under plants/).
	// Defaults to "uploads".
	UploadDir string
	// StreamDir is where stream frames are saved. Defaults to
	// <UploadDir>/streams.
	StreamDir string
	// SensorsChanged, if set, is called after sensors are added, changed
	// or deleted, so caches outside this package (the dashboard's) can be
	// dropped.
	SensorsChanged func()
}

func (d Deps) validate() error {
	switch {
	case d.DB == nil:
		return fmt.Errorf("service: DB is required")
	case d.Dialect == "":
		return fmt.Errorf("service: Dialect is required")
	case d.Clock == nil:
		return fmt.Errorf("service: Clock is required")
	case d.Zones == nil:
		return fmt.Errorf("service: Zones is required")
	case d.Store == nil:
		return fmt.Errorf("service: Store is required")
	}
	return nil
}

// Services is the entry point adapters hold. Each resource slice adds its
// service as a field here.
type Services struct {
	deps Deps

	Instance     *InstanceService
	Lookups      *LookupService
	Zones        *ZoneService
	Plants       *PlantService
	Activities   *ActivityService
	Measurements *MeasurementService
	Idempotency  *IdempotencyService
	Images       *ImageService
	Strains      *StrainService
	Breeders     *BreederService
	Sensors      *SensorService
	Streams      *StreamService
	SmokeReports *SmokeReportService
}

// New builds the services. It fails only on missing dependencies.
func New(deps Deps) (*Services, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}
	if deps.UploadDir == "" {
		deps.UploadDir = "uploads"
	}
	if deps.StreamDir == "" {
		deps.StreamDir = filepath.Join(deps.UploadDir, "streams")
	}
	s := &Services{deps: deps}
	s.Instance = &InstanceService{deps: deps}
	s.Lookups = &LookupService{deps: deps}
	s.Zones = &ZoneService{deps: deps}
	s.Plants = &PlantService{deps: deps}
	s.Activities = &ActivityService{deps: deps}
	s.Measurements = &MeasurementService{deps: deps}
	s.Idempotency = &IdempotencyService{deps: deps}
	s.Images = &ImageService{deps: deps}
	s.Strains = &StrainService{deps: deps}
	s.Breeders = &BreederService{deps: deps}
	s.Sensors = &SensorService{deps: deps}
	s.Streams = &StreamService{deps: deps}
	s.SmokeReports = &SmokeReportService{deps: deps}
	return s, nil
}

// Clock returns the clock the services use.
func (s *Services) Clock() chrono.Clock { return s.deps.Clock }

// TimeZones returns the time-zone boundary the services use.
func (s *Services) TimeZones() *chrono.Zones { return s.deps.Zones }

// UploadDir returns where uploaded files are stored.
func (s *Services) UploadDir() string { return s.deps.UploadDir }

// WithSensorsChanged returns services like s whose sensor writes call fn.
func (s *Services) WithSensorsChanged(fn func()) *Services {
	deps := s.deps
	deps.SensorsChanged = fn
	out, err := New(deps)
	if err != nil {
		// s was built from the same deps, so they are valid.
		panic(err)
	}
	return out
}
