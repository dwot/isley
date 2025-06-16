package config

import (
	"isley/model/types"
)

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
	SensorRetention    = 90 // Default sensor retention in days
	GuestMode          = 0  // Default guest mode
	StreamGrabEnabled  = 0
	StreamGrabInterval = 60
	APIKey             = ""
)
