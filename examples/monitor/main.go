package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"periph.io/x/conn/v3/i2c"
	"strconv"
	"time"

	"github.com/aldernero/scd4x"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

func main() {
	var delay int
	var count int
	useFahrenheit := flag.Bool("f", false, "Use degrees Fahrenheit (default: Celsius)")
	verboseOutput := flag.Bool("v", false, "Verbose output")
	doInit := flag.Bool("init", false, "Get sensor in state ready for measurements.")
	flag.Usage = func() {
		fmt.Printf("Usage: \n %s [options] [delay [count]]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	// Parse delay
	if flag.NArg() > 0 {
		arg, err := strconv.Atoi(flag.Arg(0))
		if err != nil {
			log.Fatal("Incorrect value for delay")
		}
		if arg < 5 {
			log.Fatal("Delay must be at least 5 seconds")
		}
		delay = arg
	}
	// Parse count
	if flag.NArg() > 1 {
		arg, err := strconv.Atoi(flag.Arg(1))
		if err != nil {
			log.Fatal("Incorrect value for count")
		}
		if arg < 1 {
			log.Fatal("Delay must be at least 1")
		}
		count = arg
	}
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
	sensor, err := scd4x.NewSensor(bus, *useFahrenheit)
	if err != nil {
		log.Fatal(err)
	}
	if *doInit {
		fmt.Print("Initializing:...")
		err := sensor.Init()
		if err != nil {
			log.Fatal(err)
		}
		err = sensor.StartMeasurements()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("done")
	}
	// Start measurements
	intervals := 0
	if *verboseOutput {
		fmt.Println("Time                            CO2   Temp    RH")
	}
	for {
		dataReady, err := sensor.GetDataReady()
		if err != nil {
			log.Fatal(err)
		}
		if !dataReady {
			log.Fatal("No data ready")
		}
		data, err := sensor.ReadMeasurement()
		if err != nil {
			log.Fatal(err)
		}
		now := time.Now()
		t := now.Format(time.RFC3339)
		d := "C"
		if sensor.UseFahrenheit {
			d = "F"
		}
		if *verboseOutput {
			fmt.Printf("[%25s] %4dppm %4.1f*%s %3.1f%%\n", t, data.CO2, data.Temp, d, data.Rh)
		} else {
			fmt.Printf("%d %.1f %.1f\n", data.CO2, data.Temp, data.Rh)
		}
		if delay == 0 {
			break
		}
		if count > 0 {
			intervals++
		}
		if intervals > count {
			break
		}
		time.Sleep(time.Duration(delay) * time.Second)
	}
}
