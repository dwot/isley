alter table plant_status add column status_order INTEGER NOT NULL DEFAULT 0;
INSERT INTO plant_status (status, active, status_order) VALUES ('Germinating', 1, 1);
INSERT INTO plant_status (status, active, status_order) VALUES ('Planted', 1, 2);
UPDATE plant_status SET status_order = 3 WHERE status = 'Seedling';
UPDATE plant_status SET status_order = 4 WHERE status = 'Veg';
UPDATE plant_status SET status_order = 5 WHERE status = 'Flower';
UPDATE plant_status SET status_order = 6 WHERE status = 'Drying';
UPDATE plant_status SET status_order = 7 WHERE status = 'Curing';
UPDATE plant_status SET status_order = 8 WHERE status = 'Success';
UPDATE plant_status SET status_order = 9 WHERE status = 'Dead';

UPDATE plant_status SET active = 0 WHERE status = 'Drying';
UPDATE plant_status SET active = 0 WHERE status = 'Curing';
