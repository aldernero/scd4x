# scd4x
A Go module for reading CO2, temperature, and humidity data from the Sesirion SCD4x family of sensors. Example sensors are the [Adafruit SCD-40 and Adafruit SCD-41](https://learn.adafruit.com/adafruit-scd-40-and-scd-41). The former was used during the development of this module.

## Scope
The module implements the following sensor functions through an i2c bus:
- Starting periodic measurements
- Reading the current sensor values
- Stopping periodic measurements

Once the sensor has started periodic measurements, it automatically records new sensor data every 5 seconds into its internal buffer. A `ReadMeasurement` call retrieves the most recent sensor data from SCD4x.

