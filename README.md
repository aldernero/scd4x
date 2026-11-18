# scd4x

A Go module for reading CO2, temperature, and humidity data from the Sensirion SCD4x family of sensors. Example sensors are the [Adafruit SCD-40 and Adafruit SCD-41](https://learn.adafruit.com/adafruit-scd-40-and-scd-41).

## Scope

The module implements the SCD4x I²C command set from the product datasheet:

| Domain | Commands |
| --- | --- |
| Basic | start/stop periodic measurement, read measurement, data ready |
| Signal compensation | get/set temperature offset, altitude, ambient pressure |
| Field calibration | forced recalibration (FRC), ASC enable/target |
| Low power | low-power periodic measurement |
| Advanced | persist settings, serial number, self-test, factory reset, reinit, sensor variant |
| Single-shot (SCD41/SCD43) | measure single shot, RHT-only, power down / wake up, ASC periods |

Once periodic measurement is running, new samples are available every 5 seconds (or 30 seconds in low-power mode). Call `GetDataReady` / `WaitForDataReady` before `ReadMeasurement`.

The SCD4x draws brief peak currents up to ~200 mA when the IR source fires. Use a supply that can handle that (datasheet recommends a dedicated LDO); an under-powered rail will look like “start works, then data never becomes ready.”

## `scd4x` configuration CLI

```
go build -o scd4x ./cmd/scd4x
./scd4x help
```

Every sensor command is available as a subcommand (`-bus` selects the I²C bus):

| Command | Description |
| --- | --- |
| `info` | variant, serial number and all settings |
| `serial`, `variant` | get serial number / sensor variant |
| `temp-offset [°C]` | get/set temperature offset |
| `altitude [m]` | get/set sensor altitude |
| `pressure [Pa]` | get/set ambient pressure |
| `frc <ppm>` | forced recalibration against a reference concentration |
| `self-test` | on-chip self test (~10 s) |
| `asc [on\|off]`, `asc-target [ppm]` | automatic self calibration |
| `asc-initial-period`, `asc-standard-period [hours]` | ASC periods (SCD41/43) |
| `persist`, `reinit`, `factory-reset -y` | EEPROM save / reload / reset |
| `start [-low-power]`, `stop` | periodic measurement |
| `single-shot [-rht-only]`, `power-down`, `wake-up` | SCD41/43 single-shot mode |

Commands that need the sensor idle stop periodic measurement first, so run `start` afterwards. Setters are volatile until you run `persist`, or pass `-persist` (e.g. `scd4x -persist temp-offset 4`) to save right after a successful set. `-persist` applies to `temp-offset`, `altitude`, `asc`, `asc-target` and the ASC periods; ambient pressure is never stored in EEPROM. Use `examples/monitor` to read measurements.

## Example CLI monitor

```
cd examples/monitor
go build -o example_monitor .
./example_monitor -h
```

```
Usage:
 ./example_monitor [options] [delay [count]]
  -bus string
    	I²C bus name (default: first available; try /dev/i2c-1)
  -f	Use degrees Fahrenheit (default: Celsius)
  -init
    	Stop, reinit, and start periodic measurements
  -v	Verbose output
```

`delay` is seconds between samples (minimum 5). `count` is how many samples to take. Omit both for a single sample. Use `-init` if you are unsure whether the sensor is already measuring.

### Output

Verbose:
```
Time                            CO2   Temp    RH
[2022-01-30T10:31:10-07:00]  801ppm 71.0*F 18.9%
```

Minimal:
```
665 71.0 18.1
```
