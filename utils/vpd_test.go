package utils

import (
	"math"
	"testing"
)

func TestFahrenheitToCelsius(t *testing.T) {
	t.Parallel()
	cases := []struct {
		f, want float64
	}{
		{32.0, 0.0},
		{212.0, 100.0},
		{77.0, 25.0},
	}
	for _, tc := range cases {
		got := FahrenheitToCelsius(tc.f)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("FahrenheitToCelsius(%v) = %v, want %v", tc.f, got, tc.want)
		}
	}
}

func TestVPD(t *testing.T) {
	t.Parallel()
	// All expected values computed from the Tetens formula by hand:
	//   es(T) = 0.6107 * 10^(7.5*T / (237.3+T))
	//   VPD   = es(airT + offset) - es(airT)*rh/100
	//
	// Case 1: 25°C, 50% RH, 0 offset
	//   es(25) = 0.6107 * 10^(187.5/262.3) ≈ 0.6107 * 10^0.7148 ≈ 0.6107 * 5.188 ≈ 3.168 kPa
	//   VPD    = 3.168 - 3.168*0.50 ≈ 1.584 kPa
	//
	// Case 2: 25°C, 70% RH, -2 offset (leaf at 23°C)
	//   es(23) = 0.6107 * 10^(172.5/260.3) ≈ 0.6107 * 10^0.6629 ≈ 0.6107 * 4.601 ≈ 2.810 kPa
	//   es(25) ≈ 3.168 kPa
	//   VPD    = 2.810 - 3.168*0.70 ≈ 2.810 - 2.218 ≈ 0.593 kPa
	//
	// Case 3: 30°C, 60% RH, 0 offset
	//   es(30) = 0.6107 * 10^(225/267.3) ≈ 0.6107 * 10^0.8419 ≈ 0.6107 * 6.950 ≈ 4.244 kPa
	//   VPD    = 4.244 - 4.244*0.60 ≈ 1.697 kPa

	cases := []struct {
		name       string
		airTempC   float64
		rhPct      float64
		leafOffset float64
		wantApprox float64
		tolerance  float64
	}{
		{"25C/50RH/0offset", 25.0, 50.0, 0.0, 1.584, 0.01},
		{"25C/70RH/-2offset", 25.0, 70.0, -2.0, 0.593, 0.01},
		{"30C/60RH/0offset", 30.0, 60.0, 0.0, 1.697, 0.01},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := VPD(tc.airTempC, tc.rhPct, tc.leafOffset)
			if math.Abs(got-tc.wantApprox) > tc.tolerance {
				t.Errorf("VPD(%v, %v, %v) = %.4f, want %.4f ± %.4f",
					tc.airTempC, tc.rhPct, tc.leafOffset, got, tc.wantApprox, tc.tolerance)
			}
		})
	}
}
