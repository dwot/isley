package utils

import (
	"math"
	"strconv"
	"strings"
)

// GramsPerOunce is the exact number of grams in an international avoirdupois
// ounce. Harvest weights are always stored in grams; ounces are a display and
// entry convenience.
const GramsPerOunce = 28.349523125

// WeightUnitGrams and WeightUnitOunces are the values stored in the
// weight_unit setting and sent by the plant forms.
const (
	WeightUnitGrams  = "g"
	WeightUnitOunces = "oz"
)

// RoundWeight keeps two decimal places, which is more precision than any
// scale used for a harvest and keeps gram/ounce round trips stable.
func RoundWeight(v float64) float64 {
	return math.Round(v*100) / 100
}

// NormalizeWeightUnit returns "oz" for ounce spellings and "g" for
// everything else, so an empty or unrecognised setting falls back to the
// canonical storage unit.
func NormalizeWeightUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "oz", "ounce", "ounces":
		return WeightUnitOunces
	default:
		return WeightUnitGrams
	}
}

// OuncesToGrams converts an ounce value to the grams stored in the database.
func OuncesToGrams(oz float64) float64 {
	return oz * GramsPerOunce
}

// GramsToOunces converts a stored gram value to ounces for display.
func GramsToOunces(g float64) float64 {
	return g / GramsPerOunce
}

// WeightToGrams converts a form value in unit to grams. Ounces are stored at
// full precision so repeated edits in the same unit do not drift; rounding is
// display-only.
func WeightToGrams(value float64, unit string) float64 {
	if NormalizeWeightUnit(unit) == WeightUnitOunces {
		return OuncesToGrams(value)
	}
	return value
}

// WeightValue returns a stored gram value expressed in unit, rounded for
// display.
func WeightValue(grams float64, unit string) float64 {
	if NormalizeWeightUnit(unit) == WeightUnitOunces {
		return RoundWeight(GramsToOunces(grams))
	}
	return RoundWeight(grams)
}

// FormatWeight renders a stored gram value in unit without a trailing unit
// symbol. A whole number prints without decimals ("2"), otherwise at most two
// decimal places are shown ("112.5").
func FormatWeight(grams float64, unit string) string {
	return strconv.FormatFloat(WeightValue(grams, unit), 'f', -1, 64)
}
