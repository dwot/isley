CREATE TABLE strain_lineage (
    id SERIAL PRIMARY KEY,
    strain_id INTEGER NOT NULL REFERENCES strain(id) ON DELETE CASCADE,
    parent_name TEXT NOT NULL,
    parent_strain_id INTEGER REFERENCES strain(id) ON DELETE SET NULL
);

CREATE INDEX idx_strain_lineage_strain_id ON strain_lineage(strain_id);
CREATE INDEX idx_strain_lineage_parent_strain_id ON strain_lineage(parent_strain_id);
