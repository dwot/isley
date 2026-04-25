package config

import (
	"isley/model/types"
	"sync/atomic"
)

// RestoreInProgress is set to true while a backup restore is running.
// Watchers check this flag and skip their iteration to avoid DB contention.
var RestoreInProgress atomic.Bool

var (
	PollingInterval    = 60 // Default polling interval in seconds
	ACIEnabled         = 0  // Default ACI enabled
	ECEnabled          = 0  // Default EC enabled
	ACIToken           = "" // Default ACI token
	ECDevices          []string
	Activities         []types.Activity
	Metrics            []types.Metric
	Statuses           []types.Status
	Zones              []types.Zone
	Strains            []types.Strain
	Breeders           []types.Breeder
	Streams            []types.Stream
	SensorRetention    = 0 // Default sensor retention in days (0 = disabled)
	GuestMode          = 0 // Default guest mode
	StreamGrabEnabled  = 0
	StreamGrabInterval = 60
	APIKey             = ""
	APIIngestEnabled   = 1                             // Default API ingest enabled
	LogLevel           = "info"                        // Default log level
	MaxBackupSize      = int64(5 * 1024 * 1024 * 1024) // Default 5 GB — configurable via settings
	Timezone           = ""                            // IANA timezone identifier (e.g. "America/New_York"); empty = system default
)
