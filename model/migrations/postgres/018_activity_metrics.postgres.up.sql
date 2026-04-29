CREATE TABLE activity_metric (
    id SERIAL PRIMARY KEY,
    activity_id INTEGER NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    metric_id INTEGER NOT NULL REFERENCES metric(id) ON DELETE CASCADE,
    required BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (activity_id, metric_id)
);

CREATE INDEX idx_activity_metric_activity_id ON activity_metric(activity_id);
CREATE INDEX idx_activity_metric_metric_id ON activity_metric(metric_id);

ALTER TABLE plant_measurements ADD COLUMN plant_activity_id INTEGER REFERENCES plant_activity(id) ON DELETE SET NULL;
CREATE INDEX idx_plant_measurements_activity_id ON plant_measurements(plant_activity_id);
