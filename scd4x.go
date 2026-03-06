package scd4x

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"periph.io/x/conn/v3/i2c"
)

// These should not change but check data sheet if in doubt
const (
	SensorAddr     uint16 = 0x62
	Crc8Polynomial uint8  = 0x31
	Crc8Init       uint8  = 0xff
)

// Supported commands
var reinitCmd = Command{
	cmd:       0x3646,
	respBytes: 0,
	delay:     30 * time.Millisecond,
	desc:      "reinitialize sensor",
}
var startCmd = Command{
	cmd:       0x21b1,
	respBytes: 0,
	delay:     0,
	desc:      "start periodic measurements",
}
var stopCmd = Command{
	cmd:       0x3f86,
	respBytes: 0,
	delay:     500 * time.Millisecond,
	desc:      "stop periodic measurements",
}
var measureCmd = Command{
	cmd:       0xec05,
	respBytes: 9,
	delay:     0,
	desc:      "read sensor metrics",
}
var temperatureOffsetCmd = Command{
	cmd:       0x2318,
	respBytes: 3,
	delay:     1 * time.Millisecond,
	desc:      "read temperature offset",
}
var sensorAltitudeCmd = Command{
	cmd:       0x2322,
	respBytes: 3,
	delay:     1 * time.Millisecond,
	desc:      "read sensor altitude compensation",
}
var ambientPressureCmd = Command{
	cmd:       0xe000,
	respBytes: 3,
	delay:     1 * time.Millisecond,
	desc:      "read ambient pressure compensation",
}
var getDataReadyCmd = Command{
	cmd:       0xe4b8,
	respBytes: 3,
	delay:     1 * time.Millisecond,
	desc:      "check if data is ready",
}

// The sensor can't handle mulitple commands at once
var mu sync.Mutex

type SensorData struct {
	CO2  uint16  // CO2 in ppm
	Temp float64 // Temperature in degrees C
	Rh   float64 // Relative humidity in %
}

type SCD4x struct {
	dev           *i2c.Dev
	data          SensorData
	debug         bool
	UseFahrenheit bool
}

type Command struct {
	cmd       uint16        // hex code from data sheet
	respBytes uint16        // expected response size (typically 0, 3, or 9)
	delay     time.Duration // time to sleep after cmd
	desc      string        // useful description for error messages
}

type Response struct {
	data []byte // expected to be two bytes
	crc  byte   // CRC8 sent by sensor of previous two bytes
}

func (r Response) CrcMatch() bool {
	return crc8(r.data, uint16(len(r.data))) == r.crc
}

func (r Response) GetData() uint16 {
	return binary.BigEndian.Uint16(r.data)
}

func NewSensor(b i2c.Bus, fahrenheit bool) (*SCD4x, error) {
	dev := &i2c.Dev{Addr: SensorAddr, Bus: b}
	return &SCD4x{dev: dev, UseFahrenheit: fahrenheit}, nil
}

func (sensor SCD4x) Init() error {
	mu.Lock()
	defer mu.Unlock()
	if err := sensor.StopMeasurements(); err != nil {
		return err
	}
	if err := sensor.sendCommand(reinitCmd); err != nil {
		return err
	}
	return nil
}

func (sensor SCD4x) StartMeasurements() error {
	//mu.Lock()
	//defer mu.Unlock()
	if err := sensor.sendCommand(startCmd); err != nil {
		return err
	}
	return nil
}

func (sensor SCD4x) StopMeasurements() error {
	//mu.Lock()
	//defer mu.Unlock()
	if err := sensor.sendCommand(stopCmd); err != nil {
		return err
	}
	return nil
}

func (sensor SCD4x) ReadMeasurement() (SensorData, error) {
	//mu.Lock()
	//defer mu.Unlock()
	var result SensorData
	resp, err := sensor.readCommand(measureCmd)
	if err != nil {
		return result, err
	}
	// check CRCs
	for _, r := range resp {
		if !r.CrcMatch() {
			return result, fmt.Errorf("measurement CRC mismatch")
		}
	}
	result = SensorData{
		CO2:  resp[0].GetData(),
		Temp: -45 + 175*float64(resp[1].GetData())/65535,
		Rh:   100 * float64(resp[2].GetData()) / 65535,
	}
	if sensor.UseFahrenheit {
		result.Temp = celsius2Fahreheit(result.Temp)
	}
	return result, nil
}

func (sensor SCD4x) GetTemperatureOffset() (float64, error) {
	//mu.Lock()
	//defer mu.Unlock()
	resp, err := sensor.readCommand(temperatureOffsetCmd)
	if err != nil {
		return 0, err
	}
	if !resp[0].CrcMatch() {
		return 0, fmt.Errorf("temperature offset CRC mismatch")
	}
	return -45 + 175*float64(resp[0].GetData())/65535, nil
}

func (sensor SCD4x) GetSensorAltitude() (uint16, error) {
	//mu.Lock()
	//defer mu.Unlock()
	resp, err := sensor.readCommand(sensorAltitudeCmd)
	if err != nil {
		return 0, err
	}
	if !resp[0].CrcMatch() {
		return 0, fmt.Errorf("sensor altitude compensation CRC mismatch")
	}
	return resp[0].GetData(), nil
}

func (sensor SCD4x) GetAmbientPressure() (uint16, error) {
	//mu.Lock()
	//defer mu.Unlock()
	resp, err := sensor.readCommand(ambientPressureCmd)
	if err != nil {
		return 0, err
	}
	if !resp[0].CrcMatch() {
		return 0, fmt.Errorf("ambient pressure compensation CRC mismatch")
	}
	return resp[0].GetData(), nil
}

func (sensor SCD4x) GetDataReady() (bool, error) {
	//mu.Lock()
	//defer mu.Unlock()
	resp, err := sensor.readCommand(getDataReadyCmd)
	if err != nil {
		return false, err
	}
	if !resp[0].CrcMatch() {
		return false, fmt.Errorf("data ready CRC mismatch")
	}
	// look at 11 least significant bits
	bits := resp[0].GetData() & 0x07ff
	return bits != 0, nil
}

// Adapted from the C/C++ example in the SDC4x data sheet
func crc8(data []byte, count uint16) byte {
	crc := Crc8Init
	for currentByte := uint16(0); currentByte < count; currentByte++ {
		crc ^= data[currentByte]
		for crcBit := 8; crcBit > 0; crcBit-- {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ Crc8Polynomial
			} else {
				crc = crc << 1
			}
		}
	}
	return crc
}

func celsius2Fahreheit(degrees float64) float64 {
	return 1.8*degrees + 32
}

func (sensor SCD4x) sendCommand(cmd Command) error {
	c := make([]byte, 2)
	binary.BigEndian.PutUint16(c, cmd.cmd)
	if err := sensor.dev.Tx(c, nil); err != nil {
		return fmt.Errorf("error while %s: %v", cmd.desc, err)
	}
	if cmd.delay > 0 {
		time.Sleep(cmd.delay)
	}
	return nil
}

func (sensor SCD4x) readCommand(cmd Command) ([]Response, error) {
	c := make([]byte, 2)
	r := make([]byte, cmd.respBytes)
	binary.BigEndian.PutUint16(c, cmd.cmd)
	if err := sensor.dev.Tx(c, r); err != nil {
		return nil, fmt.Errorf("error while %s: %v", cmd.desc, err)
	}

	resp := []Response{}
	for i := 0; i < int(cmd.respBytes)-2; i += 3 {
		j := Response{data: r[i : i+2], crc: r[i+2]}
		resp = append(resp, j)

	}
	if cmd.delay > 0 {
		time.Sleep(cmd.delay)
	}
	return resp, nil
}
