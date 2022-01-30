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
	SensorAddr               = 0x62
	StartPeriodicMeasurement = 0x21b1
	ReadMeasurement          = 0xec05
	StopPeriodicMeasurement  = 0x3f86
	CRC8_POLYNOMIAL          = byte(0x31)
	CRC8_INIT                = byte(0xff)
)

// Supported commands
var StartCommand = Command{
	cmd:       StartPeriodicMeasurement,
	respBytes: 0,
	delay:     5500 * time.Millisecond,
	desc:      "start periodic measurements",
}
var StopCommand = Command{
	cmd:       StopPeriodicMeasurement,
	respBytes: 0,
	delay:     550 * time.Millisecond,
	desc:      "stop periodic measurements",
}
var MeasureCommand = Command{
	cmd:       ReadMeasurement,
	respBytes: 9,
	delay:     0,
	desc:      "read sensor metrics",
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

func SensorInit(b i2c.Bus, fahrenheit bool) (*SCD4x, error) {
	dev := &i2c.Dev{Addr: SensorAddr, Bus: b}
	return &SCD4x{dev: dev, UseFahrenheit: fahrenheit}, nil
}

func (sensor SCD4x) StartMeasurements() error {
	mu.Lock()
	defer mu.Unlock()
	if err := sensor.sendCommand(StartCommand); err != nil {
		return err
	}
	return nil
}

func (sensor SCD4x) StopMeasurements() error {
	mu.Lock()
	defer mu.Unlock()
	if err := sensor.sendCommand(StopCommand); err != nil {
		return err
	}
	return nil
}

func (sensor SCD4x) ReadMeasurement() (SensorData, error) {
	mu.Lock()
	defer mu.Unlock()
	var result SensorData
	resp, err := sensor.readCommand(MeasureCommand)
	if err != nil {
		return result, err
	}
	// check CRCs
	for _, r := range resp {
		if !r.CrcMatch() {
			return result, fmt.Errorf("measuerment CRC mismatch")
		}
	}
	result = SensorData{
		CO2:  resp[0].GetData(),
		Temp: -45 + 175*float64(resp[1].GetData())/65536,
		Rh:   100 * float64(resp[2].GetData()) / 65536,
	}
	if sensor.UseFahrenheit {
		result.Temp = celsius2Fahreheit(result.Temp)
	}
	return result, nil
}

// Adapted from the C/C++ example in the SDC4x data sheet
func crc8(data []byte, count uint16) byte {
	crc := CRC8_INIT
	for currentByte := uint16(0); currentByte < count; currentByte++ {
		crc ^= data[currentByte]
		for crcBit := 8; crcBit > 0; crcBit-- {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ CRC8_POLYNOMIAL
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

func (sensor *SCD4x) sendCommand(cmd Command) error {
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

func (sensor *SCD4x) readCommand(cmd Command) ([]Response, error) {
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
