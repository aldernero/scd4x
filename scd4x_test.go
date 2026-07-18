package scd4x

import "testing"

func TestCRC8(t *testing.T) {
	cases := []struct {
		data []byte
		want byte
	}{
		{[]byte{0xbe, 0xef}, 0x92},
		{[]byte{0x01, 0xf4}, 0x33}, // datasheet read_measurement CO2=500
		{[]byte{0x07, 0xe6}, 0x48}, // datasheet set_temperature_offset 5.4°C
		{[]byte{0x00, 0x00}, 0x81},
	}
	for _, tc := range cases {
		if got := crc8(tc.data); got != tc.want {
			t.Errorf("crc8(%x) = 0x%02x, want 0x%02x", tc.data, got, tc.want)
		}
	}
}

func TestTemperatureOffsetConversion(t *testing.T) {
	// Datasheet example: 0x0912 → 6.2 °C
	got := offsetTicksToCelsius(0x0912)
	if got < 6.19 || got > 6.21 {
		t.Fatalf("offsetTicksToCelsius(0x0912) = %v, want ~6.2", got)
	}
	// Round-trip 5.4 °C → 0x07e6
	ticks := celsiusToOffsetTicks(5.4)
	if ticks != 0x07e6 {
		t.Fatalf("celsiusToOffsetTicks(5.4) = 0x%04x, want 0x07e6", ticks)
	}
}

func TestMeasurementConversion(t *testing.T) {
	// Datasheet example: 0x6667 → 25 °C, 0x5eb9 → 37 %RH
	temp := ticksToCelsius(0x6667)
	if temp < 24.9 || temp > 25.1 {
		t.Fatalf("ticksToCelsius(0x6667) = %v, want ~25", temp)
	}
	rh := ticksToHumidity(0x5eb9)
	if rh < 36.9 || rh > 37.1 {
		t.Fatalf("ticksToHumidity(0x5eb9) = %v, want ~37", rh)
	}
}

func TestSensorVariantString(t *testing.T) {
	if VariantSCD40.String() != "SCD40" {
		t.Fatal(VariantSCD40.String())
	}
}
