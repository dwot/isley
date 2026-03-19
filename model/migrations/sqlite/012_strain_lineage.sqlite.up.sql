CREATE TABLE strain_lineage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    strain_id INTEGER NOT NULL,
    parent_name TEXT NOT NULL,
    parent_strain_id INTEGER,
    FOREIGN KEY (strain_id) REFERENCES strain(id) ON DELETE CASCADE,
    FOREIGN KEY (parent_strain_id) REFERENCES strain(id) ON DELETE SET NULL
);

CREATE INDEX idx_strain_lineage_strain_id ON strain_lineage(strain_id);
CREATE INDEX idx_strain_lineage_parent_strain_id ON strain_lineage(parent_strain_id);
