package watcher

import "context"

// RefreshHourlyRollups summarises raw readings into sensor_data_hourly:
// everything on the first run (an empty rollup), the last 25 hours after
// that, so late readings are caught.
func (w *Watcher) RefreshHourlyRollups() error {
	if err := w.Sensors.RefreshRollup(context.Background(), false); err != nil {
		return err
	}
	w.Logger.Info("Hourly rollup refresh completed")
	return nil
}
