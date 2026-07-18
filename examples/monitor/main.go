package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aldernero/scd4x"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

func main() {
	var delay int
	var count int
	useFahrenheit := flag.Bool("f", false, "Use degrees Fahrenheit (default: Celsius)")
	verboseOutput := flag.Bool("v", false, "Verbose output")
	doInit := flag.Bool("init", false, "Stop, reinit, and start periodic measurements")
	busName := flag.String("bus", "", "I²C bus name (default: first available; try /dev/i2c-1)")
	flag.Usage = func() {
		fmt.Printf("Usage:\n %s [options] [delay [count]]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

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
	if flag.NArg() > 1 {
		arg, err := strconv.Atoi(flag.Arg(1))
		if err != nil {
			log.Fatal("Incorrect value for count")
		}
		if arg < 1 {
			log.Fatal("Count must be at least 1")
		}
		count = arg
	}

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

	sensor, err := scd4x.NewSensor(bus, *useFahrenheit)
	if err != nil {
		log.Fatal(err)
	}
	if *doInit {
		fmt.Print("Initializing...")
		if err := sensor.Init(); err != nil {
			log.Fatal(err)
		}
		if err := sensor.StartMeasurements(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("done")
	}

	intervals := 0
	var next time.Time
	if *verboseOutput {
		fmt.Println("Time                            CO2   Temp    RH")
	}
	for {
		// Allow extra time for the first sample after start (5s update interval).
		if err := sensor.WaitForDataReady(15 * time.Second); err != nil {
			log.Fatal(err)
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
		intervals++
		if delay == 0 || (count > 0 && intervals >= count) {
			break
		}
		// Pace output to the requested delay without missing the next ready sample.
		if next.IsZero() {
			next = now.Add(time.Duration(delay) * time.Second)
		} else {
			next = next.Add(time.Duration(delay) * time.Second)
		}
		if wait := time.Until(next); wait > 0 {
			time.Sleep(wait)
		}
	}
}
