// cmd/seed/main.go

package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"isley/logger"
	"isley/model"
	"isley/utils"
)

// Development fixture seeder.
//
// This creates a realistic home-grow dataset containing:
//
//   - One grow space
//   - Generated, fictional breeders (see names.go) -- one per requested
//     strain, up to maxBreeders, so every breeder has at least one strain
//   - A configurable number of strains with generated names; no real
//     breeder or brand names are used
//   - A configurable number of historical plants
//   - A realistic mixture of living, successfully completed, and dead plants
//   - Plant status history
//   - Stage-appropriate activity history
//   - Temperature and humidity sensor history
//
// Plant statuses expected by this fixture:
//
//   Germinating
//   Planted
//   Seedling
//   Veg
//   Flower
//   Drying
//   Curing
//   Success
//   Dead
//
// Plant count represents the total historical plants, not just currently
// living plants.
//
// For larger datasets the fixture aims for roughly 2–12 living plants when
// feasible, with most historical plants successfully completed and no more
// than 10% dead.
//
// The seeder is intentionally idempotent. If any plants already exist, it
// assumes the fixture has already been seeded and exits. Every other step
// (zone/breeder/strain/activity/sensor creation) is itself idempotent via
// upsertReturningID, so re-running after a partial failure is always safe.
//
// Run with:
//
//	go run ./cmd/seed
//
// Seed itself takes explicit counts rather than reading them from stdin,
// so it can also be called directly from a test (see seed_test.go).

// rng and plantNameCounters are reset at the top of every Seed call so
// repeated calls in the same process -- as happens across test cases --
// don't inherit state from a previous run.
var rng = rand.New(rand.NewSource(42))
var plantNameCounters = map[int64]int{}

func main() {
	logger.InitLogger()

	model.MigrateDB()
	model.InitDB()

	db, err := model.GetDB()
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	seeded, err := alreadySeeded(db)
	if err != nil {
		log.Fatalf("failed checking existing fixture data: %v", err)
	}

	if seeded {
		log.Println("development fixture data already exists; nothing to do")
		return
	}

	strainCount, plantCount := promptSeedCounts()

	if err := Seed(db, strainCount, plantCount); err != nil {
		log.Fatalf("seeding failed: %v", err)
	}
}

// Seed populates db with the development fixture described above. It is
// the single entry point for the fixture: main() calls it after prompting
// for counts on stdin, and tests call it directly with explicit counts.
//
// Seed re-checks alreadySeeded itself (in addition to main()'s check
// before prompting), so it's safe to call directly against a DB that's
// already been seeded -- it just becomes a no-op.
//
// Breeder and strain names are generated from rng, which is reset to a
// fixed seed on every call, so the same counts always produce the same
// fixture.
func Seed(db *sql.DB, strainCount, plantCount int) error {
	rng = rand.New(rand.NewSource(42))
	plantNameCounters = map[int64]int{}

	if strainCount < 1 {
		return fmt.Errorf("strainCount must be at least 1, got %d", strainCount)
	}

	seeded, err := alreadySeeded(db)
	if err != nil {
		return err
	}

	if seeded {
		return nil
	}

	if err := requirePlantStatuses(db); err != nil {
		return err
	}

	zoneID, err := seedZone(db, "Grow Tent")
	if err != nil {
		return err
	}

	breederNames := generateBreederNames(rng, minInt(strainCount, maxBreeders))
	definitions := generateStrains(rng, strainCount, breederNames)

	breederIDs, err := seedBreeders(db, breederNames)
	if err != nil {
		return err
	}

	strains, err := seedStrains(db, definitions, breederIDs)
	if err != nil {
		return err
	}

	if err := seedActivities(db); err != nil {
		return err
	}

	if err := seedSensorsAndData(db, zoneID); err != nil {
		return err
	}

	if err := seedPlants(db, strains, plantCount, zoneID); err != nil {
		return err
	}

	log.Println("development fixture seeding complete")

	return nil
}

func promptSeedCounts() (int, int) {
	reader := bufio.NewReader(os.Stdin)

	strainCount := promptInt(
		reader,
		"Number of strains",
		5,
		1,
		100,
	)

	plantCount := promptInt(
		reader,
		"Number of historical plants",
		6,
		1,
		1000,
	)

	return strainCount, plantCount
}

func promptInt(
	reader *bufio.Reader,
	label string,
	defaultValue int,
	minimum int,
	maximum int,
) int {
	fmt.Printf("%s [%d]: ", label, defaultValue)

	input, err := reader.ReadString('\n')
	if err != nil {
		return defaultValue
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(input)
	if err != nil || value < minimum || value > maximum {
		log.Printf(
			"invalid value %q; using default %d",
			input,
			defaultValue,
		)
		return defaultValue
	}

	return value
}

func requirePlantStatuses(db *sql.DB) error {
	required := []string{
		"Germinating",
		"Planted",
		"Seedling",
		"Veg",
		"Flower",
		"Drying",
		"Curing",
		"Success",
		"Dead",
	}

	for _, status := range required {
		var exists bool

		if err := db.QueryRow(
			`SELECT EXISTS (
				SELECT 1
				FROM plant_status
				WHERE status = $1
			)`,
			status,
		).Scan(&exists); err != nil {
			return fmt.Errorf(
				"failed checking plant_status %q: %w",
				status,
				err,
			)
		}

		if !exists {
			return fmt.Errorf(
				`required plant status %q does not exist in the database; add it to the plant_status seed/migration before running this fixture seeder`,
				status,
			)
		}
	}

	return nil
}

// alreadySeeded ties the "already seeded" decision to the presence of any
// plant row, since every other seed* helper below is itself idempotent
// (via upsertReturningID) -- seedPlants is the one step that isn't, and
// it's also the last thing Seed runs, so it's the one signal that
// actually means "seeding completed."
func alreadySeeded(db *sql.DB) (bool, error) {
	var exists bool

	if err := db.QueryRow(
		`SELECT EXISTS (
			SELECT 1
			FROM plant
		)`,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed checking existing fixture data: %w", err)
	}

	return exists, nil
}

func seedZone(db *sql.DB, name string) (int64, error) {
	return upsertReturningID(
		db,
		`INSERT INTO zones (name)
			VALUES ($1)
		  RETURNING id`,
		`SELECT id FROM zones WHERE name = $1`,
		name,
		name,
	)
}

func seedBreeders(db *sql.DB, names []string) (map[string]int64, error) {
	ids := make(map[string]int64, len(names))

	for _, name := range names {
		id, err := upsertReturningID(
			db,
			`INSERT INTO breeder (name)
				VALUES ($1)
				RETURNING id`,
			`SELECT id FROM breeder WHERE name = $1`,
			name,
			name,
		)
		if err != nil {
			return nil, err
		}

		ids[name] = id
	}

	return ids, nil
}

type strainDefinition struct {
	name       string
	breeder    string
	autoflower bool
	sativa     int
	indica     int
	cycleTime  int
}

type seededStrain struct {
	id         int64
	name       string
	autoflower bool
	cycleTime  int
}

var strainEffects = []string{
	"balanced",
	"uplifting",
	"relaxing",
	"euphoric",
	"calming",
	"focused",
}

var strainFlavors = []string{
	"citrus",
	"berry",
	"pine",
	"earth",
	"diesel",
	"tropical fruit",
	"spice",
	"floral",
}

func seedStrains(
	db *sql.DB,
	definitions []strainDefinition,
	breederIDs map[string]int64,
) ([]seededStrain, error) {
	strains := make([]seededStrain, 0, len(definitions))

	for _, definition := range definitions {
		breederID := breederIDs[definition.breeder]

		autoInt := 0
		if definition.autoflower {
			autoInt = 1
		}

		desc := fmt.Sprintf(
			"%s %s with %s, %s flavor notes.",
			strings.Title(strainEffects[rng.Intn(len(strainEffects))]),
			map[bool]string{
				true:  "autoflowering",
				false: "photoperiod",
			}[definition.autoflower],
			strings.ToLower(strainFlavors[rng.Intn(len(strainFlavors))]),
			strings.ToLower(strainFlavors[rng.Intn(len(strainFlavors))]),
		)

		seedCount := []int{3, 5, 6, 10, 12}[rng.Intn(5)]

		id, err := upsertReturningID(
			db,
			`INSERT INTO strain (
				name,
				sativa,
				indica,
				autoflower,
				description,
				seed_count,
				breeder_id,
				cycle_time
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`,
			`SELECT id FROM strain WHERE name = $1`,
			definition.name,
			definition.name,
			definition.sativa,
			definition.indica,
			autoInt,
			desc,
			seedCount,
			breederID,
			appCycleTime(definition.autoflower, definition.cycleTime),
		)
		if err != nil {
			return nil, err
		}

		strains = append(strains, seededStrain{
			id:         id,
			name:       definition.name,
			autoflower: definition.autoflower,
			cycleTime:  definition.cycleTime,
		})
	}

	return strains, nil
}

func seedActivities(db *sql.DB) error {
	activities := []string{
		"Water",
		"Feed",
		"Note",
		"Transplant",
		"Topping",
		"Training (LST)",
	}

	for _, name := range activities {
		if _, err := upsertReturningID(
			db,
			`INSERT INTO activity (name)
			 VALUES ($1)
			 RETURNING id`,
			`SELECT id FROM activity WHERE name = $1`,
			name,
			name,
		); err != nil {
			return err
		}
	}

	return nil
}

type sensorDefinition struct {
	name       string
	source     string
	device     string
	sensorType string
	unit       string
	base       float64
	amp        float64
	noise      float64
}

func seedSensorsAndData(db *sql.DB, zoneID int64) error {
	sensors := []sensorDefinition{
		{
			name:       "Govee H5075 Temperature",
			source:     "Govee",
			device:     "H5075",
			sensorType: "temperature",
			unit:       "°F",
			base:       78,
			amp:        3,
			noise:      1,
		},
		{
			name:       "Govee H5075 Humidity",
			source:     "Govee",
			device:     "H5075",
			sensorType: "humidity",
			unit:       "%",
			base:       55,
			amp:        5,
			noise:      2,
		},
	}

	for _, definition := range sensors {
		id, err := upsertReturningID(
			db,
			`INSERT INTO sensors (
		        name,
		        source,
		        device,
		        type,
		        zone_id,
		        unit,
		        visibility
		    )
		    VALUES ($1, $2, $3, $4, $5, $6, $7)
		    RETURNING id`,
			`SELECT id FROM sensors WHERE name = $1`,
			definition.name,       // keyArg
			definition.name,       // $1 name
			definition.source,     // $2 source
			definition.device,     // $3 device
			definition.sensorType, // $4 type
			zoneID,                // $5 zone_id
			definition.unit,       // $6 unit
			1,                     // $7 visibility
		)
		if err != nil {
			return err
		}

		if err := seedSensorHistory(db, id, definition); err != nil {
			return err
		}
	}

	return nil
}

func seedSensorHistory(
	db *sql.DB,
	sensorID int64,
	definition sensorDefinition,
) error {
	now := time.Now()

	for hoursAgo := 360; hoursAgo >= 0; hoursAgo-- {
		recordedAt := now.Add(-time.Duration(hoursAgo) * time.Hour)

		hourOfDay := recordedAt.Hour()
		dailyWave := math.Sin((float64(hourOfDay) - 14.0) / 24.0 * 2.0 * math.Pi)

		value := definition.base +
			definition.amp*dailyWave +
			rand.NormFloat64()*definition.noise

		if err := insertSensorData(
			db,
			sensorID,
			value,
			recordedAt,
		); err != nil {
			return fmt.Errorf("failed inserting sensor data: %w", err)
		}
	}

	return nil
}

func insertSensorData(
	db *sql.DB,
	sensorID int64,
	value float64,
	recordedAt time.Time,
) error {
	_, err := db.Exec(
		`INSERT INTO sensor_data (
			sensor_id,
			value,
			create_dt
		)
		VALUES ($1, $2, $3)`,
		sensorID,
		value,
		recordedAt.Format(utils.LayoutDB),
	)

	return err
}

// flowerStartDay is the day a plant switches to Flower: autoflowers
// flower on their own schedule, photoperiod plants after a longer veg.
func flowerStartDay(autoflower bool) int {
	if autoflower {
		return 21
	}

	return 42
}

// appCycleTime converts a strain's flowering duration into the value the
// app expects in strain.cycle_time. deriveEstimatedHarvestDate
// (handlers/plant.go) reads it as total days from start to harvest for
// autoflowers, but as days from the start of Flower to harvest for
// photoperiod strains, so the estimated harvest date lines up with the
// seeded status timeline.
func appCycleTime(autoflower bool, floweringDays int) int {
	if autoflower {
		return flowerStartDay(true) + floweringDays
	}

	return floweringDays
}

type stageOffset struct {
	status string
	day    int
}

// The plant lifecycle is represented by the actual plant_status values in
// the application:
//
//	Germinating -> Planted -> Seedling -> Veg -> Flower -> Drying -> Curing -> Success
//
// cycleTime is treated as the flowering duration, matching the strain
// definitions above.
func stageTimeline(strain seededStrain) []stageOffset {
	flowerStart := flowerStartDay(strain.autoflower)

	harvestDay := flowerStart + strain.cycleTime

	return []stageOffset{
		{
			status: "Germinating",
			day:    1,
		},
		{
			status: "Planted",
			day:    4,
		},
		{
			status: "Seedling",
			day:    7,
		},
		{
			status: "Veg",
			day:    14,
		},
		{
			status: "Flower",
			day:    flowerStart,
		},
		{
			status: "Drying",
			day:    harvestDay,
		},
		{
			status: "Curing",
			day:    harvestDay + 7,
		},
		{
			status: "Success",
			day:    harvestDay + 21,
		},
	}
}

var notesByStage = map[string][]string{
	"Germinating": {
		"Seed soaked and tucked away to germinate.",
		"Taproot showing, ready to plant soon.",
	},
	"Planted": {
		"Planted into starter medium.",
		"Tucked into the final starter cup.",
	},
	"Seedling": {
		"Second node set, growth looks vigorous.",
		"Looking healthy, no issues.",
	},
	"Veg": {
		"Vigorous growth, upped light intensity.",
		"Some yellowing on lower fan leaves, adjusted feed.",
		"Canopy filling in nicely.",
	},
	"Flower": {
		"First pistils showing.",
		"Flower development progressing normally.",
		"Trichomes developing, checking plant regularly.",
	},
	"Drying": {
		"Harvest complete, drying started.",
		"Flowers hanging to dry.",
	},
	"Curing": {
		"Moved to jars for curing.",
		"Curing underway, burping jars regularly.",
	},
	"Success": {
		"Cure complete.",
		"Finished and ready for storage.",
	},
	"Dead": {
		"Plant did not recover and was removed.",
		"Plant was lost during the grow.",
	},
}

func nextPlantName(
	db *sql.DB,
	strainID int64,
	strainName string,
) (string, error) {
	if plantNameCounters[strainID] == 0 {
		var count int

		if err := db.QueryRow(
			`SELECT COUNT(*)
			 FROM plant
			 WHERE strain_id = $1`,
			strainID,
		).Scan(&count); err != nil {
			return "", fmt.Errorf(
				"failed finding existing plant count: %w",
				err,
			)
		}

		plantNameCounters[strainID] = count
	}

	plantNameCounters[strainID]++

	return fmt.Sprintf(
		"%s #%d",
		strainName,
		plantNameCounters[strainID],
	), nil
}

func seedPlants(
	db *sql.DB,
	strains []seededStrain,
	total int,
	zoneID int64,
) error {
	if total <= 0 || len(strains) == 0 {
		return nil
	}

	livingCount := chooseLivingPlantCount(total)
	deadCount := chooseDeadPlantCount(total)
	completedCount := total - livingCount - deadCount

	if completedCount < 0 {
		completedCount = 0
	}

	log.Printf(
		"seeding %d plants: %d living, %d completed, %d dead",
		total,
		livingCount,
		completedCount,
		deadCount,
	)

	outcomes := make([]string, 0, total)

	for i := 0; i < livingCount; i++ {
		outcomes = append(outcomes, "living")
	}

	for i := 0; i < completedCount; i++ {
		outcomes = append(outcomes, "success")
	}

	for i := 0; i < deadCount; i++ {
		outcomes = append(outcomes, "dead")
	}

	rng.Shuffle(
		len(outcomes),
		func(i, j int) {
			outcomes[i], outcomes[j] = outcomes[j], outcomes[i]
		},
	)

	for i, outcome := range outcomes {
		strain := strains[rng.Intn(len(strains))]

		var err error

		switch outcome {
		case "living":
			err = seedLivingPlant(
				db,
				strain,
				zoneID,
			)

		case "success":
			err = seedCompletedPlant(
				db,
				strain,
				zoneID,
			)

		case "dead":
			err = seedDeadPlant(
				db,
				strain,
				zoneID,
			)
		}

		if err != nil {
			return err
		}

		if (i+1)%25 == 0 || i == len(outcomes)-1 {
			log.Printf(
				"seeded %d/%d plants",
				i+1,
				len(outcomes),
			)
		}
	}

	return nil
}

func chooseLivingPlantCount(total int) int {
	if total <= 2 {
		return total
	}

	if total <= 4 {
		return total - 1
	}

	maxLiving := total
	if maxLiving > 12 {
		maxLiving = 12
	}

	minLiving := 2
	if minLiving > maxLiving {
		minLiving = maxLiving
	}

	return minLiving + rng.Intn(maxLiving-minLiving+1)
}

func chooseDeadPlantCount(total int) int {
	if total < 10 {
		if rng.Float64() < 0.15 {
			return 1
		}

		return 0
	}

	maxDead := total / 10
	if maxDead < 1 {
		return 0
	}

	return 1 + rng.Intn(maxDead)
}

func seedLivingPlant(
	db *sql.DB,
	strain seededStrain,
	zoneID int64,
) error {
	timeline := stageTimeline(strain)

	flowerStart := timelineDay(
		timeline,
		"Flower",
	)

	var ageDays int

	stageChoice := rng.Intn(3)

	switch stageChoice {
	case 0:
		ageDays = 7 + rng.Intn(7)

	case 1:
		ageDays = flowerStart - 1
		if ageDays < 14 {
			ageDays = 14
		}

		ageDays = 14 + rng.Intn(
			maxInt(1, flowerStart-14),
		)

	case 2:
		ageDays = flowerStart +
			rng.Intn(maxInt(1, strain.cycleTime))
	}

	startDate := time.Now().AddDate(
		0,
		0,
		-ageDays,
	)

	return seedOnePlant(
		db,
		strain,
		zoneID,
		startDate,
		"living",
		0,
	)
}

func seedCompletedPlant(
	db *sql.DB,
	strain seededStrain,
	zoneID int64,
) error {
	timeline := stageTimeline(strain)

	successDay := timelineDay(
		timeline,
		"Success",
	)

	// Keep completed grows historical without making every record ancient.
	// A completed plant is at least about a month old and can be several
	// months old.
	daysSinceSuccess := 30 + rng.Intn(181)

	totalAge := successDay + daysSinceSuccess

	startDate := time.Now().AddDate(
		0,
		0,
		-totalAge,
	)

	return seedOnePlant(
		db,
		strain,
		zoneID,
		startDate,
		"success",
		successDay,
	)
}

func seedDeadPlant(
	db *sql.DB,
	strain seededStrain,
	zoneID int64,
) error {
	timeline := stageTimeline(strain)

	flowerStart := timelineDay(
		timeline,
		"Flower",
	)

	var deathDay int

	stageChoice := rng.Intn(4)

	switch stageChoice {
	case 0:
		// Early loss, shortly after germinating.
		deathDay = 1 + rng.Intn(3)

	case 1:
		deathDay = 7 + rng.Intn(7)

	case 2:
		deathDay = 14 + rng.Intn(
			maxInt(1, flowerStart-14),
		)

	case 3:
		deathDay = flowerStart +
			rng.Intn(
				maxInt(1, strain.cycleTime/2),
			)
	}

	daysSinceDeath := rng.Intn(30)

	totalAge := deathDay + daysSinceDeath

	startDate := time.Now().AddDate(
		0,
		0,
		-totalAge,
	)

	return seedOnePlant(
		db,
		strain,
		zoneID,
		startDate,
		"dead",
		deathDay,
	)
}

func seedOnePlant(
	db *sql.DB,
	strain seededStrain,
	zoneID int64,
	startDate time.Time,
	outcome string,
	explicitEndDay int,
) error {
	name, err := nextPlantName(
		db,
		strain.id,
		strain.name,
	)
	if err != nil {
		return err
	}

	description := fmt.Sprintf(
		"%s grown in the Grow Tent.",
		strain.name,
	)

	plantID, err := insertPlant(
		db,
		name,
		description,
		strain.id,
		zoneID,
		startDate,
	)
	if err != nil {
		return err
	}

	ageDays := int(
		time.Since(startDate).Hours() / 24,
	)

	endDay := ageDays

	if explicitEndDay > 0 {
		endDay = explicitEndDay
	}

	timeline := stageTimeline(strain)

	for _, stage := range timeline {
		if stage.day > endDay {
			break
		}

		if err := insertPlantStatusLog(
			db,
			plantID,
			stage.status,
			startDate.AddDate(
				0,
				0,
				stage.day,
			),
		); err != nil {
			return err
		}
	}

	if outcome == "dead" {
		if err := insertPlantStatusLog(
			db,
			plantID,
			"Dead",
			startDate.AddDate(
				0,
				0,
				endDay,
			),
		); err != nil {
			return err
		}
	}

	return seedPlantActivityHistory(
		db,
		plantID,
		startDate,
		endDay,
		outcome,
		timeline,
	)
}

func insertPlant(
	db *sql.DB,
	name string,
	description string,
	strainID int64,
	zoneID int64,
	startDate time.Time,
) (int64, error) {
	var id int64

	err := db.QueryRow(
		`INSERT INTO plant (
			name,
			description,
			clone,
			strain_id,
			zone_id,
			start_dt
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		name,
		description,
		0,
		strainID,
		zoneID,
		startDate.Format(utils.LayoutDate),
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf(
			"failed inserting plant %q: %w",
			name,
			err,
		)
	}

	return id, nil
}

func insertPlantStatusLog(
	db *sql.DB,
	plantID int64,
	status string,
	statusDate time.Time,
) error {
	var statusID int64

	err := db.QueryRow(
		`SELECT id
		 FROM plant_status
		 WHERE status = $1`,
		status,
	).Scan(&statusID)

	if err != nil {
		return fmt.Errorf(
			"failed looking up plant status %q: %w",
			status,
			err,
		)
	}

	var exists bool

	if err := db.QueryRow(
		`SELECT EXISTS (
			SELECT 1
			FROM plant_status_log
			WHERE plant_id = $1
			  AND status_id = $2
		)`,
		plantID,
		statusID,
	).Scan(&exists); err != nil {
		return fmt.Errorf(
			"failed checking plant status history for plant %d: %w",
			plantID,
			err,
		)
	}

	if exists {
		return nil
	}

	if _, err := db.Exec(
		`INSERT INTO plant_status_log (
			plant_id,
			status_id,
			date
		)
		VALUES ($1, $2, $3)`,
		plantID,
		statusID,
		statusDate.Format(utils.LayoutDate),
	); err != nil {
		return fmt.Errorf(
			"failed inserting plant status %q for plant %d: %w",
			status,
			plantID,
			err,
		)
	}

	return nil
}

func seedPlantActivityHistory(
	db *sql.DB,
	plantID int64,
	startDate time.Time,
	endDay int,
	outcome string,
	timeline []stageOffset,
) error {
	var existing int

	if err := db.QueryRow(
		`SELECT COUNT(*)
		 FROM plant_activity
		 WHERE plant_id = $1`,
		plantID,
	).Scan(&existing); err != nil {
		return fmt.Errorf(
			"failed checking activity history for plant %d: %w",
			plantID,
			err,
		)
	}

	if existing > 0 {
		return nil
	}

	vegStartDay := timelineDay(
		timeline,
		"Veg",
	)

	for day := 1; day <= endDay; day++ {
		date := startDate.AddDate(
			0,
			0,
			day,
		)

		stage := stageAt(
			timeline,
			day,
		)

		// Water only while the plant is actively growing.
		if day%3 == 0 &&
			(stage == "Seedling" ||
				stage == "Veg" ||
				stage == "Flower") {
			if err := insertActivity(
				db,
				plantID,
				"Water",
				date,
				"Watered according to routine.",
			); err != nil {
				return err
			}
		}

		// Feed weekly during vegetative growth and flowering.
		if day%7 == 0 &&
			(stage == "Veg" || stage == "Flower") {
			if err := insertActivity(
				db,
				plantID,
				"Feed",
				date,
				"Fed according to the current schedule.",
			); err != nil {
				return err
			}
		}

		// First transplant.
		if day == 7 && endDay >= 7 {
			if err := insertActivity(
				db,
				plantID,
				"Transplant",
				date,
				"Moved into a larger container.",
			); err != nil {
				return err
			}
		}

		// Final container transplant.
		if day == vegStartDay && day > 7 {
			if err := insertActivity(
				db,
				plantID,
				"Transplant",
				date,
				"Moved into the final container.",
			); err != nil {
				return err
			}
		}

		// Training is more common during early/mid veg.
		if stage == "Veg" && day == vegStartDay+7 &&
			rng.Float64() < 0.5 {
			if err := insertActivity(
				db,
				plantID,
				"Topping",
				date,
				"Main growth topped to encourage a wider canopy.",
			); err != nil {
				return err
			}
		}

		if stage == "Veg" && day == vegStartDay+10 &&
			rng.Float64() < 0.5 {
			if err := insertActivity(
				db,
				plantID,
				"Training (LST)",
				date,
				"Applied low-stress training to open the canopy.",
			); err != nil {
				return err
			}
		}

		// Occasional free-form observations.
		if rng.Float64() < 0.10 {
			if notes, ok := notesByStage[stage]; ok &&
				len(notes) > 0 {
				if err := insertActivity(
					db,
					plantID,
					"Note",
					date,
					notes[rng.Intn(len(notes))],
				); err != nil {
					return err
				}
			}
		}
	}

	switch outcome {
	case "success":
		successDay := timelineDay(
			timeline,
			"Success",
		)

		if successDay <= endDay {
			notes := notesByStage["Success"]

			if err := insertActivity(
				db,
				plantID,
				"Note",
				startDate.AddDate(
					0,
					0,
					successDay,
				),
				notes[rng.Intn(len(notes))],
			); err != nil {
				return err
			}
		}

	case "dead":
		if endDay > 0 {
			notes := notesByStage["Dead"]

			if err := insertActivity(
				db,
				plantID,
				"Note",
				startDate.AddDate(
					0,
					0,
					endDay,
				),
				notes[rng.Intn(len(notes))],
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func stageAt(
	timeline []stageOffset,
	day int,
) string {
	current := timeline[0].status

	for _, stage := range timeline {
		if stage.day > day {
			break
		}

		current = stage.status
	}

	return current
}

// timelineDay looks up a status's day offset in a timeline built by
// stageTimeline. It stays a hard failure (not an error return) rather
// than propagating through the Seed() error chain like everything else
// here: stageTimeline is a fixed literal that always includes every
// status this is ever called with, so a miss here means the seeder's own
// code is broken, not that something DB- or environment-dependent went
// wrong. That can't happen differently between a production run and a
// test run, so there's no testability reason to turn it into an error.
func timelineDay(
	timeline []stageOffset,
	status string,
) int {
	for _, stage := range timeline {
		if stage.status == status {
			return stage.day
		}
	}

	log.Fatalf(
		"status %q does not exist in plant timeline",
		status,
	)

	return 0
}

func insertActivity(
	db *sql.DB,
	plantID int64,
	activityName string,
	activityDate time.Time,
	note string,
) error {
	var activityID int64

	if err := db.QueryRow(
		`SELECT id
		 FROM activity
		 WHERE name = $1`,
		activityName,
	).Scan(&activityID); err != nil {
		return fmt.Errorf(
			"failed looking up activity %q: %w",
			activityName,
			err,
		)
	}

	if _, err := db.Exec(
		`INSERT INTO plant_activity (
			plant_id,
			activity_id,
			date,
			note
		)
		VALUES ($1, $2, $3, $4)`,
		plantID,
		activityID,
		activityDate.Format(utils.LayoutDate),
		note,
	); err != nil {
		return fmt.Errorf(
			"failed inserting %q activity for plant %d: %w",
			activityName,
			plantID,
			err,
		)
	}

	return nil
}

func upsertReturningID(
	db *sql.DB,
	insertQuery string,
	lookupQuery string,
	keyArg interface{},
	insertArgs ...interface{},
) (int64, error) {
	var id int64

	err := db.QueryRow(
		lookupQuery,
		keyArg,
	).Scan(&id)

	if err == nil {
		return id, nil
	}

	if err != sql.ErrNoRows {
		return 0, fmt.Errorf(
			"failed looking up existing record: %w",
			err,
		)
	}

	if err := db.QueryRow(
		insertQuery,
		insertArgs...,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf(
			"failed inserting record: %w",
			err,
		)
	}

	return id, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
