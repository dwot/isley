package service

// StatusName is a plant status as stored in plant_status.status. The set is
// fixed by migrations (users can't add or rename statuses), so these names
// are safe to rely on. This file is the one place that says what each
// status means; code elsewhere asks these questions instead of comparing
// names.
type StatusName string

const (
	StatusGerminating StatusName = "Germinating"
	StatusPlanted     StatusName = "Planted"
	StatusSeedling    StatusName = "Seedling"
	StatusVeg         StatusName = "Veg"
	StatusFlower      StatusName = "Flower"
	StatusDrying      StatusName = "Drying"
	StatusCuring      StatusName = "Curing"
	StatusSuccess     StatusName = "Success"
	StatusDead        StatusName = "Dead"
)

// IsGrowing reports whether a plant in this status is still growing
// (plant_status.active = 1).
func (s StatusName) IsGrowing() bool {
	switch s {
	case StatusGerminating, StatusPlanted, StatusSeedling, StatusVeg, StatusFlower:
		return true
	}
	return false
}

// IsHarvested reports whether the plant has been cut down and is drying,
// curing or done.
func (s StatusName) IsHarvested() bool {
	switch s {
	case StatusDrying, StatusCuring, StatusSuccess:
		return true
	}
	return false
}

// IsDead reports whether the plant died.
func (s StatusName) IsDead() bool { return s == StatusDead }

// EndsGrowth reports whether entering this status ends the plant's growth:
// harvested or dead. The first such status in a plant's history is its
// harvest (or death) day, and flowering time stops counting there.
func (s StatusName) EndsGrowth() bool { return s.IsHarvested() || s.IsDead() }

// IsFlowering reports whether the plant is in flower.
func (s StatusName) IsFlowering() bool { return s == StatusFlower }

// allStatuses is every status in lifecycle order (plant_status.status_order).
var allStatuses = []StatusName{
	StatusGerminating, StatusPlanted, StatusSeedling, StatusVeg, StatusFlower,
	StatusDrying, StatusCuring, StatusSuccess, StatusDead,
}

// AllStatuses returns every status in lifecycle order.
func AllStatuses() []StatusName {
	return append([]StatusName(nil), allStatuses...)
}
