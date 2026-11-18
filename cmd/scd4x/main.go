// Command scd4x configures and queries a Sensirion SCD4x sensor from the
// command line. Run "scd4x help" for the list of subcommands.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aldernero/scd4x"
	"periph.io/x/conn/v3/i2c"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/host/v3"
)

type command struct {
	name  string
	args  string
	desc  string
	idle  bool // sensor must be idle; periodic measurement is stopped first
	saves bool // setting is stored in EEPROM, so -persist applies
	run   func(s *scd4x.SCD4x, args []string) error
}

var commands []command

func init() {
	commands = []command{
		{name: "info", desc: "show variant, serial number and all settings", idle: true, run: cmdInfo},
		{name: "serial", desc: "get serial number", idle: true, run: cmdSerial},
		{name: "variant", desc: "get sensor variant (SCD40/41/42/43)", idle: true, run: cmdVariant},
		{name: "temp-offset", args: "[celsius]", desc: "get/set temperature offset in °C (0-175)", idle: true, saves: true, run: cmdTempOffset},
		{name: "altitude", args: "[meters]", desc: "get/set sensor altitude in m (0-65535)", idle: true, saves: true, run: cmdAltitude},
		{name: "pressure", args: "[pascals]", desc: "get/set ambient pressure in Pa (70000-120000)", run: cmdPressure},
		{name: "frc", args: "<ppm>", desc: "forced recalibration against a reference CO2 concentration", idle: true, run: cmdFRC},
		{name: "self-test", desc: "run the on-chip self test (takes ~10 s)", idle: true, run: cmdSelfTest},
		{name: "asc", args: "[on|off]", desc: "get/set automatic self calibration", idle: true, saves: true, run: cmdASC},
		{name: "asc-target", args: "[ppm]", desc: "get/set ASC target CO2 concentration", idle: true, saves: true, run: cmdASCTarget},
		{name: "asc-initial-period", args: "[hours]", desc: "get/set ASC initial period (SCD41/43, multiple of 4)", idle: true, saves: true, run: cmdASCInitial},
		{name: "asc-standard-period", args: "[hours]", desc: "get/set ASC standard period (SCD41/43, multiple of 4)", idle: true, saves: true, run: cmdASCStandard},
		{name: "persist", desc: "save current settings to EEPROM", idle: true, run: cmdPersist},
		{name: "reinit", desc: "reload settings from EEPROM", idle: true, run: cmdReinit},
		{name: "factory-reset", args: "[-y]", desc: "reset all settings and calibration to factory defaults", idle: true, run: cmdFactoryReset},
		{name: "start", args: "[-low-power]", desc: "start periodic measurement (5 s, or 30 s with -low-power)", run: cmdStart},
		{name: "stop", desc: "stop periodic measurement", run: cmdStop},
		{name: "single-shot", args: "[-rht-only]", desc: "one on-demand measurement (SCD41/43)", idle: true, run: cmdSingleShot},
		{name: "power-down", desc: "put the sensor to sleep (SCD41/43)", idle: true, run: cmdPowerDown},
		{name: "wake-up", desc: "wake the sensor from sleep (SCD41/43)", run: cmdWakeUp},
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage:\n  %s [-bus name] <command> [args]\n\nCommands:\n", os.Args[0])
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-20s %-12s %s\n", c.name, c.args, c.desc)
	}
	fmt.Fprint(os.Stderr, `
Commands that require the idle state stop any running periodic measurement
first; restart it with "start". Setters change volatile settings only; add
-persist (or run "persist") to keep them across power cycles.

Options:
`)
	flag.PrintDefaults()
}

func main() {
	busName := flag.String("bus", "", "I²C bus name (default: first available; try /dev/i2c-1)")
	persist := flag.Bool("persist", false, "after setting a value, save it to EEPROM (temp-offset, altitude, asc*; not pressure)")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() == 0 || flag.Arg(0) == "help" {
		usage()
		if flag.NArg() == 0 {
			os.Exit(2)
		}
		return
	}
	name, args := flag.Arg(0), flag.Args()[1:]

	var cmd *command
	for i := range commands {
		if commands[i].name == name {
			cmd = &commands[i]
		}
	}
	if cmd == nil {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", name)
		usage()
		os.Exit(2)
	}
	if *persist && !cmd.saves {
		fmt.Fprintf(os.Stderr, "error: -persist does not apply to %q (only temp-offset, altitude, asc, asc-target, asc-*-period; ambient pressure is never stored in EEPROM)\n", name)
		os.Exit(2)
	}
	if err := run(*busName, cmd, args, *persist && len(args) > 0); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(busName string, cmd *command, args []string, persist bool) (err error) {
	if _, err := host.Init(); err != nil {
		return fmt.Errorf("initialize periph: %w", err)
	}
	bus, err := i2creg.Open(busName)
	if err != nil {
		return fmt.Errorf("open bus: %w", err)
	}
	defer func(bus i2c.BusCloser) {
		if cerr := bus.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close bus: %w", cerr)
		}
	}(bus)

	s, err := scd4x.NewSensor(bus, false)
	if err != nil {
		return err
	}
	if cmd.idle {
		// The sensor NACKs stop when already idle; that is not an error here.
		_ = s.StopMeasurements()
	}
	if err := cmd.run(s, args); err != nil {
		return err
	}
	if persist {
		return s.PersistSettings()
	}
	return nil
}

// --- argument helpers ---

func needArgs(args []string, min, max int, usage string) error {
	if len(args) < min || len(args) > max {
		return fmt.Errorf("usage: %s", usage)
	}
	return nil
}

func parseUint(s string, bits int, what string) (uint64, error) {
	v, err := strconv.ParseUint(s, 0, bits)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q (want integer 0-%d)", what, s, uint64(1)<<uint(bits)-1)
	}
	return v, nil
}

// --- commands ---

func cmdInfo(s *scd4x.SCD4x, _ []string) error {
	variant, err := s.GetSensorVariant()
	if err != nil {
		return err
	}
	serial, err := s.GetSerialNumber()
	if err != nil {
		return err
	}
	off, err := s.GetTemperatureOffset()
	if err != nil {
		return err
	}
	alt, err := s.GetSensorAltitude()
	if err != nil {
		return err
	}
	pa, err := s.GetAmbientPressure()
	if err != nil {
		return err
	}
	asc, err := s.GetAutomaticSelfCalibrationEnabled()
	if err != nil {
		return err
	}
	target, err := s.GetAutomaticSelfCalibrationTarget()
	if err != nil {
		return err
	}
	fmt.Printf("variant:             %s\n", variant)
	fmt.Printf("serial:              0x%012x\n", serial)
	fmt.Printf("temperature offset:  %.2f °C\n", off)
	fmt.Printf("altitude:            %d m\n", alt)
	fmt.Printf("ambient pressure:    %d Pa\n", pa)
	fmt.Printf("asc:                 %s\n", onOff(asc))
	fmt.Printf("asc target:          %d ppm\n", target)
	if variant == scd4x.VariantSCD41 || variant == scd4x.VariantSCD43 {
		ini, err := s.GetAutomaticSelfCalibrationInitialPeriod()
		if err != nil {
			return err
		}
		std, err := s.GetAutomaticSelfCalibrationStandardPeriod()
		if err != nil {
			return err
		}
		fmt.Printf("asc initial period:  %d h\n", ini)
		fmt.Printf("asc standard period: %d h\n", std)
	}
	return nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func cmdSerial(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "serial"); err != nil {
		return err
	}
	v, err := s.GetSerialNumber()
	if err != nil {
		return err
	}
	fmt.Printf("0x%012x\n", v)
	return nil
}

func cmdVariant(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "variant"); err != nil {
		return err
	}
	v, err := s.GetSensorVariant()
	if err != nil {
		return err
	}
	fmt.Println(v)
	return nil
}

func cmdTempOffset(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 1, "temp-offset [celsius]"); err != nil {
		return err
	}
	if len(args) == 1 {
		v, err := strconv.ParseFloat(args[0], 64)
		if err != nil || v < 0 || v > 175 {
			return fmt.Errorf("invalid temperature offset %q (want 0-175 °C)", args[0])
		}
		return s.SetTemperatureOffset(v)
	}
	v, err := s.GetTemperatureOffset()
	if err != nil {
		return err
	}
	fmt.Printf("%.2f\n", v)
	return nil
}

func cmdAltitude(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 1, "altitude [meters]"); err != nil {
		return err
	}
	if len(args) == 1 {
		v, err := parseUint(args[0], 16, "altitude")
		if err != nil {
			return err
		}
		return s.SetSensorAltitude(uint16(v))
	}
	v, err := s.GetSensorAltitude()
	if err != nil {
		return err
	}
	fmt.Println(v)
	return nil
}

func cmdPressure(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 1, "pressure [pascals]"); err != nil {
		return err
	}
	if len(args) == 1 {
		v, err := parseUint(args[0], 32, "pressure")
		if err != nil {
			return err
		}
		return s.SetAmbientPressure(uint32(v))
	}
	v, err := s.GetAmbientPressure()
	if err != nil {
		return err
	}
	fmt.Println(v)
	return nil
}

func cmdFRC(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 1, 1, "frc <ppm>"); err != nil {
		return err
	}
	ppm, err := parseUint(args[0], 16, "target CO2")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "note: the sensor must have run in periodic mode for >3 minutes in the target environment before FRC")
	corr, err := s.PerformForcedRecalibration(uint16(ppm))
	if err != nil {
		return err
	}
	fmt.Printf("correction: %+d ppm\n", corr)
	return nil
}

func cmdSelfTest(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "self-test"); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "running self test (~10 s)...")
	if err := s.PerformSelfTest(); err != nil {
		return err
	}
	fmt.Println("self test passed")
	return nil
}

func cmdASC(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 1, "asc [on|off]"); err != nil {
		return err
	}
	if len(args) == 1 {
		switch args[0] {
		case "on":
			return s.SetAutomaticSelfCalibrationEnabled(true)
		case "off":
			return s.SetAutomaticSelfCalibrationEnabled(false)
		}
		return fmt.Errorf("invalid value %q (want on or off)", args[0])
	}
	v, err := s.GetAutomaticSelfCalibrationEnabled()
	if err != nil {
		return err
	}
	fmt.Println(onOff(v))
	return nil
}

func uint16Cmd(name string, set func(uint16) error, get func() (uint16, error)) func(*scd4x.SCD4x, []string) error {
	return func(_ *scd4x.SCD4x, args []string) error {
		if err := needArgs(args, 0, 1, name+" [value]"); err != nil {
			return err
		}
		if len(args) == 1 {
			v, err := parseUint(args[0], 16, "value")
			if err != nil {
				return err
			}
			return set(uint16(v))
		}
		v, err := get()
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	}
}

func cmdASCTarget(s *scd4x.SCD4x, args []string) error {
	return uint16Cmd("asc-target", s.SetAutomaticSelfCalibrationTarget, s.GetAutomaticSelfCalibrationTarget)(s, args)
}

func cmdASCInitial(s *scd4x.SCD4x, args []string) error {
	return uint16Cmd("asc-initial-period", s.SetAutomaticSelfCalibrationInitialPeriod, s.GetAutomaticSelfCalibrationInitialPeriod)(s, args)
}

func cmdASCStandard(s *scd4x.SCD4x, args []string) error {
	return uint16Cmd("asc-standard-period", s.SetAutomaticSelfCalibrationStandardPeriod, s.GetAutomaticSelfCalibrationStandardPeriod)(s, args)
}

func cmdPersist(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "persist"); err != nil {
		return err
	}
	return s.PersistSettings()
}

func cmdReinit(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "reinit"); err != nil {
		return err
	}
	return s.Reinit()
}

func cmdFactoryReset(s *scd4x.SCD4x, args []string) error {
	fs := flag.NewFlagSet("factory-reset", flag.ContinueOnError)
	yes := fs.Bool("y", false, "confirm the reset")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*yes {
		return fmt.Errorf("factory-reset erases all settings and calibration; rerun with -y to confirm")
	}
	return s.PerformFactoryReset()
}

func cmdStart(s *scd4x.SCD4x, args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	low := fs.Bool("low-power", false, "30 s measurement interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *low {
		return s.StartLowPowerPeriodicMeasurement()
	}
	return s.StartPeriodicMeasurement()
}

func cmdStop(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "stop"); err != nil {
		return err
	}
	return s.StopMeasurements()
}

func cmdSingleShot(s *scd4x.SCD4x, args []string) error {
	fs := flag.NewFlagSet("single-shot", flag.ContinueOnError)
	rht := fs.Bool("rht-only", false, "measure temperature and humidity only (fast, CO2 reads 0)")
	f := fs.Bool("f", false, "Fahrenheit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s.UseFahrenheit = *f
	var err error
	if *rht {
		err = s.MeasureSingleShotRHTOnly()
	} else {
		err = s.MeasureSingleShot()
	}
	if err != nil {
		return err
	}
	if err := s.WaitForDataReady(10 * time.Second); err != nil {
		return err
	}
	d, err := s.ReadMeasurement()
	if err != nil {
		return err
	}
	fmt.Printf("%d %.1f %.1f\n", d.CO2, d.Temp, d.Rh)
	return nil
}

func cmdPowerDown(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "power-down"); err != nil {
		return err
	}
	return s.PowerDown()
}

func cmdWakeUp(s *scd4x.SCD4x, args []string) error {
	if err := needArgs(args, 0, 0, "wake-up"); err != nil {
		return err
	}
	return s.WakeUp()
}
