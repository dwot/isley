package main

import (
	"fmt"
	"math/rand"
)

// Fixture names are generated from the word lists below instead of being
// taken from real breeders, so the seeder doesn't tie any real brand to
// this project. Strain names are built from generic, descriptive words in
// the style of classic strain naming ("White Widow", "Sour Purple Haze");
// breeder names are invented compounds. Avoid adding brand names,
// trademarked words, or the names of real breeders to these lists.
//
// Every function here takes its own *rand.Rand, so output is
// reproducible for a given seed and independent of the package-level rng.

// maxBreeders caps how many breeders are generated. Seed generates
// min(strainCount, maxBreeders) breeders so that every breeder ends up
// with at least one strain.
const maxBreeders = 30

var strainColors = []string{
	"White", "Black", "Blue", "Purple", "Golden",
	"Pink", "Crimson", "Silver", "Orange", "Green",
	"Red",
}

var strainModifiers = []string{
	"Sour", "Sweet", "Frosted", "Wild", "Midnight",
	"Electric", "Northern", "Velvet", "Sunset", "Sticky",
	"Mountain", "Lazy", "Smokey", "Royal", "Toasted",
	"Icy", "Exotic",
}

var strainFruits = []string{
	"Mango", "Lemon", "Cherry", "Peach", "Grape",
	"Papaya", "Guava", "Plum", "Lime", "Tangerine",
	"Raspberry", "Apricot", "Melon", "Pomelo", "Kiwi",
}

var strainNouns = []string{
	"Widow", "Haze", "Kush", "Skunk", "Diesel",
	"Dream", "Cookie", "Punch", "Cake", "Sherbet",
	"Thunder", "Mist", "Fire", "Crush", "Rocket",
	"Lightning", "Storm", "Pie", "Fog", "Cooler",
	"Mint", "Drip", "Gumdrop", "Socks",
}

var breederPrefixes = []string{
	"Copper", "Fern", "Quill", "Moss", "Ember",
	"Drift", "Alder", "Thistle", "Juniper", "Marrow",
	"Saffron", "Lantern", "Harbor", "Cinder", "Willow",
	"Bramble", "Sable", "Kettle", "Pebble", "Wren",
	"Tamarack", "Nettle", "Sorrel", "Flint", "Bracken",
}

var breederEndings = []string{
	"line", "hollow", "wick", "brook", "stead",
	"field", "ford", "gate", "well", "crest",
	"mere", "thorn", "vale", "wood", "haven",
	"dale", "street",
}

var breederSuffixes = []string{
	"Seed Co.", "Genetics", "Seeds", "Seed Bank",
	"Botanicals", "Cultivars", "Selections", "Garden",
}

func pick(r *rand.Rand, words []string) string {
	return words[r.Intn(len(words))]
}

// generateStrainName builds a name from one of four patterns, e.g.
// "White Widow", "Sour Purple Haze", "Frosted Cookie", "Mango Kush".
func generateStrainName(r *rand.Rand) string {
	switch r.Intn(4) {
	case 0:
		return pick(r, strainColors) + " " + pick(r, strainNouns)
	case 1:
		return pick(r, strainModifiers) + " " +
			pick(r, strainColors) + " " +
			pick(r, strainNouns)
	case 2:
		return pick(r, strainModifiers) + " " + pick(r, strainNouns)
	default:
		return pick(r, strainFruits) + " " + pick(r, strainNouns)
	}
}

// generateBreederNames returns count unique invented breeder names such as
// "Copperwick Seed Co." or "Fernhollow Genetics".
func generateBreederNames(r *rand.Rand, count int) []string {
	names := make([]string, 0, count)
	seen := make(map[string]bool, count)

	for attempts := 0; len(names) < count; attempts++ {
		name := pick(r, breederPrefixes) +
			pick(r, breederEndings) + " " +
			pick(r, breederSuffixes)

		// The word lists allow several hundred distinct names, far more
		// than maxBreeders. If a caller ever asks for more than they can
		// produce, number the extras instead of looping forever.
		if seen[name] {
			if attempts < count*50 {
				continue
			}

			name = fmt.Sprintf("%s %d", name, len(names)+1)
		}

		seen[name] = true
		names = append(names, name)
	}

	return names
}

// generateStrains returns count strains with unique names, assigned to
// breeders round-robin. With at least as many strains as breeders, every
// breeder gets at least one.
func generateStrains(
	r *rand.Rand,
	count int,
	breeders []string,
) []strainDefinition {
	definitions := make([]strainDefinition, 0, count)
	seen := make(map[string]bool, count)

	for i := 0; i < count; i++ {
		autoflower := r.Float64() < 0.35

		name := uniqueStrainName(r, seen, autoflower)

		sativa := 10 + 5*r.Intn(17)

		cycleTime := []int{56, 63, 70}[r.Intn(3)]
		if autoflower {
			cycleTime = 35
		}

		definitions = append(definitions, strainDefinition{
			name:       name,
			breeder:    breeders[i%len(breeders)],
			autoflower: autoflower,
			sativa:     sativa,
			indica:     100 - sativa,
			cycleTime:  cycleTime,
		})
	}

	return definitions
}

// uniqueStrainName keeps generating until it finds a name not already in
// seen. Strains are looked up by name, so a repeat would silently collapse
// two requested strains into one row.
func uniqueStrainName(
	r *rand.Rand,
	seen map[string]bool,
	autoflower bool,
) string {
	for attempts := 0; ; attempts++ {
		name := generateStrainName(r)

		if autoflower {
			name += " Auto"
		}

		if attempts >= 1000 {
			name = fmt.Sprintf("%s #%d", name, len(seen)+1)
		}

		if !seen[name] {
			seen[name] = true
			return name
		}
	}
}
