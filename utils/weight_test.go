package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeWeightUnit(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":          WeightUnitGrams,
		"g":         WeightUnitGrams,
		"G":         WeightUnitGrams,
		"grams":     WeightUnitGrams,
		"oz":        WeightUnitOunces,
		"OZ":        WeightUnitOunces,
		" ounce ":   WeightUnitOunces,
		"Ounces":    WeightUnitOunces,
		"kilograms": WeightUnitGrams,
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeWeightUnit(in), "NormalizeWeightUnit(%q)", in)
	}
}

func TestWeightConversionRoundTrip(t *testing.T) {
	t.Parallel()

	// One ounce is stored as 28.349523125 g and displays back as 1 oz.
	grams := WeightToGrams(1, WeightUnitOunces)
	assert.InDelta(t, 28.349523125, grams, 1e-9)
	assert.InDelta(t, 1, GramsToOunces(grams), 0.0001)
	assert.Equal(t, "1", FormatWeight(grams, WeightUnitOunces))

	// A two-ounce entry survives the round trip.
	grams = WeightToGrams(2, WeightUnitOunces)
	assert.InDelta(t, 56.69904625, grams, 1e-6)
	assert.Equal(t, "2", FormatWeight(grams, WeightUnitOunces))

	// A typical gram harvest keeps its value.
	grams = WeightToGrams(112.5, WeightUnitGrams)
	assert.Equal(t, 112.5, grams)
	assert.Equal(t, "112.5", FormatWeight(grams, WeightUnitGrams))
	assert.Equal(t, "3.97", FormatWeight(grams, WeightUnitOunces))
}

func TestFormatWeightUnknownUnitIsGrams(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "112.5", FormatWeight(112.5, ""))
	assert.Equal(t, "112.5", FormatWeight(112.5, "stone"))
}
