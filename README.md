# scd4x
A Go module for reading CO2, temperature, and humidity data from the Sesirion SCD4x family of sensors. Example sensors are the [Adafruit SCD-40 and Adafruit SCD-41](https://learn.adafruit.com/adafruit-scd-40-and-scd-41). The former was used during the development of this module.

## Scope
The module implements the following sensor functions through an i2c bus:
- Starting periodic measurements
- Reading the current sensor values
- Stopping periodic measurements

Once the sensor has started periodic measurements, it automatically records new sensor data every 5 seconds into its internal buffer. A `ReadMeasurement` call retrieves the most recent sensor data from SCD4x.

## Example CLI Monitor

The `example_monitor.go` provides a similar CLI monitor with syntax similar to other CLI tools like `iostat` and `vmstat`. 

### Installation

Clone this repository and navigate to the `examples/` directory. Then build with go.
```
go build example_monitor.go
```
You should now have an executable file called `example_monitor`

### Syntax

```
pi@sliceofpi:~/scd4x/examples $ ./example_monitor -h
Usage:
 ./example_monitor [options] [delay [count]]
  -f	Use degrees Fahrenheit (default: Celsius)
  -init
    	Get sensor in state ready for measurements.
  -v	Verbose output
```
Like other system tools, `delay` is the number of seconds to wait between measurements, and `count` is the total number of measurements to take. Both field are optional. Not specifying the delay will take one measurement and exit. Specifying a delay but not a count will run the monitor indefinitely, until it receives an interrupt (e.g. Ctrl+C).

If you're not sure what state the sensor is in, i.e. whether its in an idle state or already in a periodic measurement state, you can use the `--init` flag, which will issue a `StopMeasurement` followed by a `StartMeasurement` command. The `--init` operation takes about 7 seconds to complete. You should only ever need to use the flag once.

### Output
Verbose output:
```
pi@sliceofpi:~/scd4x/examples $ ./example_monitor -f -v 10
Time                            CO2   Temp    RH
[2022-01-30T10:31:10-07:00]  801ppm 71.0*F 18.9%
[2022-01-30T10:31:20-07:00]  782ppm 70.9*F 19.0%
[2022-01-30T10:31:30-07:00]  780ppm 70.9*F 19.0%
[2022-01-30T10:31:40-07:00]  780ppm 70.9*F 18.9%
[2022-01-30T10:31:50-07:00]  780ppm 70.9*F 18.9%
```
Minimal output:
```
pi@sliceofpi:~/scd4x/examples $ ./example_monitor -f 10
665 71.0 18.1
665 71.0 18.1
663 71.0 18.0
663 71.0 18.0
663 71.0 18.0
663 71.1 18.0
```