package main

import (
	"flag"
	"log"

	"github.com/aldernero/scd4x"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

func main() {
	busName := flag.String("bus", "", "I²C bus name (default: first available; try /dev/i2c-1)")
	flag.Parse()

	if _, err := host.Init(); err != nil {
		log.Fatalf("Failed to initialize periph: %v", err)
	}
	bus, err := i2creg.Open(*busName)
	if err != nil {
		log.Fatalf("Failed while opening bus: %v", err)
	}
	defer func(bus i2c.BusCloser) {
		if err := bus.Close(); err != nil {
			log.Fatal("Failed to close bus: ", err)
		}
	}(bus)

	sensor, err := scd4x.NewSensor(bus, false)
	if err != nil {
		log.Fatal("Failed to create sensor: ", err)
	}
	if err := sensor.Init(); err != nil {
		log.Fatalf("Failed to initialize sensor: %v", err)
	}

	variant, err := sensor.GetSensorVariant()
	if err != nil {
		log.Fatal("Failed to get sensor variant: ", err)
	}
	serial, err := sensor.GetSerialNumber()
	if err != nil {
		log.Fatal("Failed to get serial number: ", err)
	}
	tempOffset, err := sensor.GetTemperatureOffset()
	if err != nil {
		log.Fatal("Failed to get temperature offset: ", err)
	}
	sensorAltitude, err := sensor.GetSensorAltitude()
	if err != nil {
		log.Fatal("Failed to get sensor altitude compensation: ", err)
	}
	ambientPressure, err := sensor.GetAmbientPressure()
	if err != nil {
		log.Fatal("Failed to get ambient pressure compensation: ", err)
	}
	ascEnabled, err := sensor.GetAutomaticSelfCalibrationEnabled()
	if err != nil {
		log.Fatal("Failed to get ASC enabled: ", err)
	}
	ascTarget, err := sensor.GetAutomaticSelfCalibrationTarget()
	if err != nil {
		log.Fatal("Failed to get ASC target: ", err)
	}

	log.Printf("Variant: %s", variant)
	log.Printf("Serial number: 0x%012x", serial)
	log.Printf("Temperature offset: %.2f °C", tempOffset)
	log.Printf("Sensor altitude: %d m", sensorAltitude)
	log.Printf("Ambient pressure: %d Pa (%.0f hPa)", ambientPressure, float64(ambientPressure)/100)
	log.Printf("ASC enabled: %v (target %d ppm)", ascEnabled, ascTarget)
}
