DELETE FROM activity WHERE name = 'note';

CREATE TABLE plant_note (
                            id INTEGER PRIMARY KEY AUTOINCREMENT,
                            plant_id INTEGER NOT NULL,
                            note TEXT NOT NULL,
                            date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                            FOREIGN KEY (plant_id) REFERENCES plant(id)
);

