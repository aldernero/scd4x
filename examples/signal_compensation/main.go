package main

import (
	"github.com/aldernero/scd4x"
	"log"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

func main() {
	// Prepare I2C bus
	_, err := host.Init()
	if err != nil {
		log.Fatalf("Failed to initialize periph: %v", err)
	}
	bus, err := i2creg.Open("")
	if err != nil {
		log.Fatalf("Failed while opening bus: %v", err)
	}
	defer func(bus i2c.BusCloser) {
		err := bus.Close()
		if err != nil {
			log.Fatal("Failed to close bus: ", err)
		}
	}(bus)
	// Initialize sensor
	sensor, err := scd4x.NewSensor(bus, false)
	if err != nil {
		log.Fatal("Failed to initialize sensor: ", err)
	}
	// Get temperature offset
	tempOffset, err := sensor.GetTemperatureOffset()
	if err != nil {
		log.Fatal("Failed to get temperature offset: ", err)
	}
	// Get sensor altitude compensation
	sensorAltitude, err := sensor.GetSensorAltitude()
	if err != nil {
		log.Fatal("Failed to get sensor altitude compensation: ", err)
	}
	// Get ambient pressure compensation
	ambientPressure, err := sensor.GetAmbientPressure()
	if err != nil {
		log.Fatal("Failed to get ambient pressure compensation: ", err)
	}
	// Print results
	log.Printf("Temperature offset (degrees C): %v", tempOffset)
	log.Printf("Sensor altitude (meters): %v", sensorAltitude)
	log.Printf("Ambient pressure (hPa): %v", ambientPressure)
}
