package service

import (
	"context"
	"time"
)

// Instance describes the running Isley so clients can adapt to it.
type Instance struct {
	Version string
	// TimeZone is the IANA name of the instance zone: the zone API
	// timestamps are given in and the one naive inputs are read in.
	TimeZone string
	// Now is the server's current time in the instance zone.
	Now       time.Time
	GuestMode bool
	Features  InstanceFeatures
}

// InstanceFeatures says which optional parts of Isley are switched on.
type InstanceFeatures struct {
	ACInfinity    bool
	EcoWitt       bool
	SensorIngest  bool
	CannaDB       bool
	StreamCapture bool
	MCP           bool
}

// InstanceService reports facts about the running instance.
type InstanceService struct {
	deps Deps
}

// Get returns the instance description.
func (s *InstanceService) Get(ctx context.Context) (Instance, error) {
	if err := ctx.Err(); err != nil {
		return Instance{}, err
	}
	store := s.deps.Store
	return Instance{
		Version:   s.deps.Version,
		TimeZone:  s.deps.Zones.InstanceName(),
		Now:       s.deps.Zones.Now(s.deps.Clock),
		GuestMode: store.GuestMode() == 1,
		Features: InstanceFeatures{
			ACInfinity:    store.ACIEnabled() == 1,
			EcoWitt:       store.ECEnabled() == 1,
			SensorIngest:  store.APIIngestEnabled() == 1,
			CannaDB:       store.CannadbEnabled() == 1,
			StreamCapture: store.StreamGrabEnabled() == 1,
			MCP:           store.MCPEnabled() == 1,
		},
	}, nil
}
