-- Smoke reports: post-harvest tasting notes attached to a plant.
--
-- A report belongs to a plant, not a strain: two harvests of the same genetics
-- differ with the grow, the dry and the cure, and averaging them at strain level
-- would erase exactly the information worth keeping. Several reports per plant
-- are expected — a two-week cure and a three-month cure describe two different
-- products, which is what report_date records.
--
-- Criteria are user-configurable from Settings. They are soft-deleted via the
-- active flag rather than removed: unlike a plant height, a tasting impression
-- cannot be re-measured, so dropping a criterion must never destroy the scores
-- already recorded against it. plant_status uses the same active-flag pattern.
CREATE TABLE smoke_report_criterion (
                          id SERIAL PRIMARY KEY,
                          name TEXT NOT NULL,
                          sort_order INTEGER NOT NULL DEFAULT 0,
                          active INTEGER NOT NULL DEFAULT 1
);

INSERT INTO smoke_report_criterion (name, sort_order) VALUES
    ('Aroma', 1), ('Taste', 2), ('Effect', 3), ('Potency', 4), ('Burn', 5);

CREATE TABLE smoke_report (
                          id SERIAL PRIMARY KEY,
                          plant_id INTEGER NOT NULL,
                          report_date TIMESTAMP NOT NULL,
                          consumption_method TEXT NOT NULL DEFAULT '',
                          rating INTEGER NOT NULL,
                          description TEXT NOT NULL DEFAULT '',
                          created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          FOREIGN KEY (plant_id) REFERENCES plant(id) ON DELETE CASCADE
);

CREATE INDEX idx_smoke_report_plant_id ON smoke_report(plant_id);

-- The CASCADE on criterion_id is an integrity backstop, not a nominal path:
-- no handler hard-deletes a criterion that any score references (it archives
-- instead). See handlers/smoke_report_criterion.go DeleteSmokeCriterionHandler.
CREATE TABLE smoke_report_score (
                          id SERIAL PRIMARY KEY,
                          report_id INTEGER NOT NULL,
                          criterion_id INTEGER NOT NULL,
                          value INTEGER NOT NULL,
                          UNIQUE (report_id, criterion_id),
                          FOREIGN KEY (report_id) REFERENCES smoke_report(id) ON DELETE CASCADE,
                          FOREIGN KEY (criterion_id) REFERENCES smoke_report_criterion(id) ON DELETE CASCADE
);

CREATE INDEX idx_smoke_report_score_report_id ON smoke_report_score(report_id);
