package main

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateBreederNames_UniqueAndDeterministic(t *testing.T) {
	t.Parallel()

	first := generateBreederNames(rand.New(rand.NewSource(1)), maxBreeders)
	second := generateBreederNames(rand.New(rand.NewSource(1)), maxBreeders)

	require.Len(t, first, maxBreeders)
	require.Equal(t, first, second, "same seed should give the same names")

	seen := map[string]bool{}
	for _, name := range first {
		require.False(t, seen[name], "duplicate breeder %q", name)
		seen[name] = true
	}
}

func TestGenerateStrains_UniqueNamesAndValidAttributes(t *testing.T) {
	t.Parallel()

	breeders := generateBreederNames(rand.New(rand.NewSource(1)), maxBreeders)
	definitions := generateStrains(rand.New(rand.NewSource(1)), 100, breeders)

	require.Len(t, definitions, 100)

	seen := map[string]bool{}
	for _, d := range definitions {
		// Strains are looked up by name, so a repeat would silently
		// merge two requested strains into one row.
		require.False(t, seen[d.name], "duplicate strain %q", d.name)
		seen[d.name] = true

		require.Equal(t, 100, d.sativa+d.indica, "%q sativa+indica", d.name)
		require.Positive(t, d.cycleTime, "%q cycle time", d.name)
	}
}

// Round-robin assignment means that once there are at least as many
// strains as breeders, no breeder is left without a strain.
func TestGenerateStrains_EveryBreederGetsAStrain(t *testing.T) {
	t.Parallel()

	breeders := generateBreederNames(rand.New(rand.NewSource(1)), 12)
	definitions := generateStrains(rand.New(rand.NewSource(1)), 12, breeders)

	assigned := map[string]bool{}
	for _, d := range definitions {
		assigned[d.breeder] = true
	}

	require.Len(t, assigned, len(breeders))
}

func TestGenerateStrains_AutoflowerNamesAreMarked(t *testing.T) {
	t.Parallel()

	breeders := generateBreederNames(rand.New(rand.NewSource(1)), 5)

	for _, d := range generateStrains(rand.New(rand.NewSource(1)), 100, breeders) {
		if d.autoflower {
			require.Equal(t, 35, d.cycleTime)
			require.True(t, len(d.name) > 5 && d.name[len(d.name)-5:] == " Auto",
				"autoflower %q should end in \" Auto\"", d.name)
		}
	}
}
