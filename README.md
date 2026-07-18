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
