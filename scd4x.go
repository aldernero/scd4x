// Package scd4x drives Sensirion SCD40/SCD41/SCD43 CO2, temperature, and
// humidity sensors over I²C (address 0x62).
//
// Command codes, timings, and conversions follow the SCD4x datasheet.
package scd4x

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"periph.io/x/conn/v3/i2c"
)

const (
	SensorAddr     uint16 = 0x62
	Crc8Polynomial uint8  = 0x31
	Crc8Init       uint8  = 0xff

	// 2^16 - 1, used by datasheet signal conversions.
	signalScale = 65535.0
)

// I²C command codes (datasheet Table 9).
const (
	cmdStartPeriodicMeasurement                  uint16 = 0x21b1
	cmdReadMeasurement                           uint16 = 0xec05
	cmdStopPeriodicMeasurement                   uint16 = 0x3f86
	cmdSetTemperatureOffset                      uint16 = 0x241d
	cmdGetTemperatureOffset                      uint16 = 0x2318
	cmdSetSensorAltitude                         uint16 = 0x2427
	cmdGetSensorAltitude                         uint16 = 0x2322
	cmdSetAmbientPressure                        uint16 = 0xe000
	cmdGetAmbientPressure                        uint16 = 0xe000
	cmdPerformForcedRecalibration                uint16 = 0x362f
	cmdSetAutomaticSelfCalibrationEnabled        uint16 = 0x2416
	cmdGetAutomaticSelfCalibrationEnabled        uint16 = 0x2313
	cmdSetAutomaticSelfCalibrationTarget         uint16 = 0x243a
	cmdGetAutomaticSelfCalibrationTarget         uint16 = 0x233f
	cmdStartLowPowerPeriodicMeasurement          uint16 = 0x21ac
	cmdGetDataReadyStatus                        uint16 = 0xe4b8
	cmdPersistSettings                           uint16 = 0x3615
	cmdGetSerialNumber                           uint16 = 0x3682
	cmdPerformSelfTest                           uint16 = 0x3639
	cmdPerformFactoryReset                       uint16 = 0x3632
	cmdReinit                                    uint16 = 0x3646
	cmdGetSensorVariant                          uint16 = 0x202f
	cmdMeasureSingleShot                         uint16 = 0x219d
	cmdMeasureSingleShotRHTOnly                  uint16 = 0x2196
	cmdPowerDown                                 uint16 = 0x36e0
	cmdWakeUp                                    uint16 = 0x36f6
	cmdSetAutomaticSelfCalibrationInitialPeriod  uint16 = 0x2445
	cmdGetAutomaticSelfCalibrationInitialPeriod  uint16 = 0x2340
	cmdSetAutomaticSelfCalibrationStandardPeriod uint16 = 0x244e
	cmdGetAutomaticSelfCalibrationStandardPeriod uint16 = 0x234b
)

// Max command execution times from the datasheet.
const (
	delayRead         = 1 * time.Millisecond
	delayWrite        = 1 * time.Millisecond
	delayStop         = 500 * time.Millisecond
	delayForcedRecal  = 400 * time.Millisecond
	delayPersist      = 800 * time.Millisecond
	delaySelfTest     = 10 * time.Second
	delayFactoryReset = 1200 * time.Millisecond
	// Table 9 lists 20 ms; Table 7 soft-reset time after reinit is 1000 ms.
	delayReinit        = 1000 * time.Millisecond
	delaySingleShot    = 5 * time.Second
	delaySingleShotRHT = 50 * time.Millisecond
	delayPowerDown     = 1 * time.Millisecond
	delayWakeUp        = 30 * time.Millisecond
)

var (
	ErrFRCFailed      = errors.New("forced recalibration failed (sensor not operated long enough)")
	ErrSelfTestFailed = errors.New("self-test detected a malfunction")
	ErrDataNotReady   = errors.New("measurement data not ready")
)

// SensorVariant identifies the SCD4x product variant.
type SensorVariant uint16

const (
	VariantUnknown SensorVariant = 0xffff
	VariantSCD40   SensorVariant = 0x0000
	VariantSCD41   SensorVariant = 0x1000
	VariantSCD42   SensorVariant = 0x2000
	VariantSCD43   SensorVariant = 0x5000

	variantMask uint16 = 0xf000
)

func (v SensorVariant) String() string {
	switch v {
	case VariantSCD40:
		return "SCD40"
	case VariantSCD41:
		return "SCD41"
	case VariantSCD42:
		return "SCD42"
	case VariantSCD43:
		return "SCD43"
	default:
		return fmt.Sprintf("unknown(0x%04x)", uint16(v))
	}
}

// SensorData holds one CO2 / temperature / humidity sample.
type SensorData struct {
	CO2  uint16  // CO2 in ppm
	Temp float64 // Temperature in °C, or °F when UseFahrenheit is set
	Rh   float64 // Relative humidity in %
}

// SCD4x is a handle to a sensor on an I²C bus.
type SCD4x struct {
	dev           *i2c.Dev
	mu            sync.Mutex
	UseFahrenheit bool

	// Reused to avoid per-transaction allocations on the measurement path.
	cmdBuf  [5]byte
	respBuf [9]byte
}

// NewSensor creates a sensor handle. Measurements are not started automatically.
func NewSensor(b i2c.Bus, fahrenheit bool) (*SCD4x, error) {
	if b == nil {
		return nil, errors.New("i2c bus is nil")
	}
	return &SCD4x{
		dev:           &i2c.Dev{Addr: SensorAddr, Bus: b},
		UseFahrenheit: fahrenheit,
	}, nil
}

// Init stops any running measurement and reinitializes the sensor into idle mode.
// A failing stop is ignored because the sensor NACKs stop when already idle.
func (s *SCD4x) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.stopMeasurementsLocked()
	return s.sendLocked(cmdReinit, delayReinit, "reinitialize sensor")
}

// StartMeasurements starts periodic measurement (5 s update interval).
func (s *SCD4x) StartMeasurements() error {
	return s.StartPeriodicMeasurement()
}

// StartPeriodicMeasurement starts periodic measurement (5 s update interval).
func (s *SCD4x) StartPeriodicMeasurement() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdStartPeriodicMeasurement, 0, "start periodic measurement")
}

// StartLowPowerPeriodicMeasurement starts low-power periodic measurement (30 s interval).
func (s *SCD4x) StartLowPowerPeriodicMeasurement() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdStartLowPowerPeriodicMeasurement, 0, "start low power periodic measurement")
}

// StopMeasurements stops periodic measurement and returns the sensor to idle.
func (s *SCD4x) StopMeasurements() error {
	return s.StopPeriodicMeasurement()
}

// StopPeriodicMeasurement stops periodic measurement and returns the sensor to idle.
func (s *SCD4x) StopPeriodicMeasurement() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopMeasurementsLocked()
}

func (s *SCD4x) stopMeasurementsLocked() error {
	return s.sendLocked(cmdStopPeriodicMeasurement, delayStop, "stop periodic measurement")
}

// GetDataReady reports whether a new measurement is available.
func (s *SCD4x) GetDataReady() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetDataReadyStatus, delayRead, 1, "check data ready")
	if err != nil {
		return false, err
	}
	return words[0]&0x07ff != 0, nil
}

// WaitForDataReady polls GetDataReady until data is ready, ctx-style via timeout.
func (s *SCD4x) WaitForDataReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		ready, err := s.GetDataReady()
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrDataNotReady
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ReadMeasurement reads the latest CO2, temperature, and humidity sample.
// The internal buffer is emptied on read; call GetDataReady first to avoid a NACK.
func (s *SCD4x) ReadMeasurement() (SensorData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result SensorData
	words, err := s.readWordsLocked(cmdReadMeasurement, delayRead, 3, "read measurement")
	if err != nil {
		return result, err
	}
	result = SensorData{
		CO2:  words[0],
		Temp: ticksToCelsius(words[1]),
		Rh:   ticksToHumidity(words[2]),
	}
	if s.UseFahrenheit {
		result.Temp = celsiusToFahrenheit(result.Temp)
	}
	return result, nil
}

// SetTemperatureOffset sets the temperature offset in °C (idle mode only).
func (s *SCD4x) SetTemperatureOffset(celsius float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeWordLocked(cmdSetTemperatureOffset, celsiusToOffsetTicks(celsius), delayWrite, "set temperature offset")
}

// GetTemperatureOffset returns the configured temperature offset in °C (idle mode only).
func (s *SCD4x) GetTemperatureOffset() (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetTemperatureOffset, delayRead, 1, "get temperature offset")
	if err != nil {
		return 0, err
	}
	return offsetTicksToCelsius(words[0]), nil
}

// SetSensorAltitude sets altitude compensation in meters (idle mode only).
func (s *SCD4x) SetSensorAltitude(meters uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeWordLocked(cmdSetSensorAltitude, meters, delayWrite, "set sensor altitude")
}

// GetSensorAltitude returns the configured altitude compensation in meters.
func (s *SCD4x) GetSensorAltitude() (uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetSensorAltitude, delayRead, 1, "get sensor altitude")
	if err != nil {
		return 0, err
	}
	return words[0], nil
}

// SetAmbientPressure sets ambient pressure compensation in Pascals.
// Valid range is 70_000–120_000 Pa. May be called during periodic measurement.
func (s *SCD4x) SetAmbientPressure(pa uint32) error {
	if pa < 70000 || pa > 120000 {
		return fmt.Errorf("ambient pressure %d Pa out of range 70000-120000", pa)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeWordLocked(cmdSetAmbientPressure, uint16((pa+50)/100), delayWrite, "set ambient pressure")
}

// GetAmbientPressure returns the configured ambient pressure in Pascals.
func (s *SCD4x) GetAmbientPressure() (uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetAmbientPressure, delayRead, 1, "get ambient pressure")
	if err != nil {
		return 0, err
	}
	return uint32(words[0]) * 100, nil
}

// PerformForcedRecalibration runs FRC against targetCO2 (ppm) and returns the
// applied correction in ppm. Sensor must have been measuring for ≥3 minutes first.
func (s *SCD4x) PerformForcedRecalibration(targetCO2 uint16) (int16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.writeReadWordsLocked(cmdPerformForcedRecalibration, targetCO2, delayForcedRecal, 1, "perform forced recalibration")
	if err != nil {
		return 0, err
	}
	if words[0] == 0xffff {
		return 0, ErrFRCFailed
	}
	return int16(int32(words[0]) - 0x8000), nil
}

// SetAutomaticSelfCalibrationEnabled enables or disables ASC (idle mode only).
func (s *SCD4x) SetAutomaticSelfCalibrationEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v uint16
	if enabled {
		v = 1
	}
	return s.writeWordLocked(cmdSetAutomaticSelfCalibrationEnabled, v, delayWrite, "set ASC enabled")
}

// GetAutomaticSelfCalibrationEnabled reports whether ASC is enabled.
func (s *SCD4x) GetAutomaticSelfCalibrationEnabled() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetAutomaticSelfCalibrationEnabled, delayRead, 1, "get ASC enabled")
	if err != nil {
		return false, err
	}
	return words[0] == 1, nil
}

// SetAutomaticSelfCalibrationTarget sets the ASC target CO2 concentration in ppm.
func (s *SCD4x) SetAutomaticSelfCalibrationTarget(ppm uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeWordLocked(cmdSetAutomaticSelfCalibrationTarget, ppm, delayWrite, "set ASC target")
}

// GetAutomaticSelfCalibrationTarget returns the ASC target CO2 concentration in ppm.
func (s *SCD4x) GetAutomaticSelfCalibrationTarget() (uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetAutomaticSelfCalibrationTarget, delayRead, 1, "get ASC target")
	if err != nil {
		return 0, err
	}
	return words[0], nil
}

// PersistSettings saves volatile configuration to EEPROM (idle mode only).
func (s *SCD4x) PersistSettings() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdPersistSettings, delayPersist, "persist settings")
}

// GetSerialNumber returns the 48-bit sensor serial number.
func (s *SCD4x) GetSerialNumber() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetSerialNumber, delayRead, 3, "get serial number")
	if err != nil {
		return 0, err
	}
	return uint64(words[0])<<32 | uint64(words[1])<<16 | uint64(words[2]), nil
}

// PerformSelfTest runs the on-chip self-test (takes ~10 s; idle mode only).
func (s *SCD4x) PerformSelfTest() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdPerformSelfTest, delaySelfTest, 1, "perform self-test")
	if err != nil {
		return err
	}
	if words[0] != 0 {
		return fmt.Errorf("%w: status=0x%04x", ErrSelfTestFailed, words[0])
	}
	return nil
}

// PerformFactoryReset resets all configuration to factory defaults (idle mode only).
func (s *SCD4x) PerformFactoryReset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdPerformFactoryReset, delayFactoryReset, "perform factory reset")
}

// Reinit reloads settings from EEPROM into volatile memory (idle mode only).
func (s *SCD4x) Reinit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdReinit, delayReinit, "reinitialize sensor")
}

// GetSensorVariant returns the product variant (SCD40 / SCD41 / SCD42 / SCD43).
func (s *SCD4x) GetSensorVariant() (SensorVariant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetSensorVariant, delayRead, 1, "get sensor variant")
	if err != nil {
		return VariantUnknown, err
	}
	switch SensorVariant(words[0] & variantMask) {
	case VariantSCD40:
		return VariantSCD40, nil
	case VariantSCD41:
		return VariantSCD41, nil
	case VariantSCD42:
		return VariantSCD42, nil
	case VariantSCD43:
		return VariantSCD43, nil
	default:
		return SensorVariant(words[0] & variantMask), nil
	}
}

// MeasureSingleShot triggers a single-shot CO2/T/RH measurement (SCD41/SCD43 only).
// After this returns, call ReadMeasurement.
func (s *SCD4x) MeasureSingleShot() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdMeasureSingleShot, delaySingleShot, "measure single shot")
}

// MeasureSingleShotRHTOnly triggers a single-shot T/RH measurement (SCD41/SCD43 only).
// CO2 is returned as 0. After this returns, call ReadMeasurement.
func (s *SCD4x) MeasureSingleShotRHTOnly() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdMeasureSingleShotRHTOnly, delaySingleShotRHT, "measure single shot RHT only")
}

// PowerDown puts the sensor to sleep (SCD41/SCD43 single-shot power cycling).
func (s *SCD4x) PowerDown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sendLocked(cmdPowerDown, delayPowerDown, "power down")
}

// WakeUp wakes the sensor from sleep. The sensor does not ACK this command.
func (s *SCD4x) WakeUp() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putCommand(cmdWakeUp)
	_ = s.dev.Tx(s.cmdBuf[:2], nil) // NACK is expected
	time.Sleep(delayWakeUp)
	return nil
}

// SetAutomaticSelfCalibrationInitialPeriod sets the ASC initial period in hours
// (multiples of 4; idle mode; SCD41/SCD43 single-shot tuning).
func (s *SCD4x) SetAutomaticSelfCalibrationInitialPeriod(hours uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeWordLocked(cmdSetAutomaticSelfCalibrationInitialPeriod, hours, delayWrite, "set ASC initial period")
}

// GetAutomaticSelfCalibrationInitialPeriod returns the ASC initial period in hours.
func (s *SCD4x) GetAutomaticSelfCalibrationInitialPeriod() (uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetAutomaticSelfCalibrationInitialPeriod, delayRead, 1, "get ASC initial period")
	if err != nil {
		return 0, err
	}
	return words[0], nil
}

// SetAutomaticSelfCalibrationStandardPeriod sets the ASC standard period in hours
// (multiples of 4; idle mode; SCD41/SCD43 single-shot tuning).
func (s *SCD4x) SetAutomaticSelfCalibrationStandardPeriod(hours uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeWordLocked(cmdSetAutomaticSelfCalibrationStandardPeriod, hours, delayWrite, "set ASC standard period")
}

// GetAutomaticSelfCalibrationStandardPeriod returns the ASC standard period in hours.
func (s *SCD4x) GetAutomaticSelfCalibrationStandardPeriod() (uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, err := s.readWordsLocked(cmdGetAutomaticSelfCalibrationStandardPeriod, delayRead, 1, "get ASC standard period")
	if err != nil {
		return 0, err
	}
	return words[0], nil
}

// --- low-level I²C ---

func (s *SCD4x) putCommand(cmd uint16) {
	binary.BigEndian.PutUint16(s.cmdBuf[0:2], cmd)
}

func (s *SCD4x) putCommandWord(cmd, word uint16) {
	s.putCommand(cmd)
	binary.BigEndian.PutUint16(s.cmdBuf[2:4], word)
	s.cmdBuf[4] = crc8(s.cmdBuf[2:4])
}

func (s *SCD4x) sendLocked(cmd uint16, delay time.Duration, desc string) error {
	s.putCommand(cmd)
	if err := s.dev.Tx(s.cmdBuf[:2], nil); err != nil {
		return fmt.Errorf("%s: %w", desc, err)
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return nil
}

func (s *SCD4x) writeWordLocked(cmd, word uint16, delay time.Duration, desc string) error {
	s.putCommandWord(cmd, word)
	if err := s.dev.Tx(s.cmdBuf[:5], nil); err != nil {
		return fmt.Errorf("%s: %w", desc, err)
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return nil
}

// readWordsLocked performs a datasheet "read" sequence: write command, wait, then read.
func (s *SCD4x) readWordsLocked(cmd uint16, delay time.Duration, nWords int, desc string) ([]uint16, error) {
	s.putCommand(cmd)
	if err := s.dev.Tx(s.cmdBuf[:2], nil); err != nil {
		return nil, fmt.Errorf("%s: write: %w", desc, err)
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	n := nWords * 3
	buf := s.respBuf[:n]
	if err := s.dev.Tx(nil, buf); err != nil {
		return nil, fmt.Errorf("%s: read: %w", desc, err)
	}
	return parseWords(buf, nWords, desc)
}

// writeReadWordsLocked performs "send command and fetch result" (e.g. FRC).
func (s *SCD4x) writeReadWordsLocked(cmd, word uint16, delay time.Duration, nWords int, desc string) ([]uint16, error) {
	s.putCommandWord(cmd, word)
	if err := s.dev.Tx(s.cmdBuf[:5], nil); err != nil {
		return nil, fmt.Errorf("%s: write: %w", desc, err)
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	n := nWords * 3
	buf := s.respBuf[:n]
	if err := s.dev.Tx(nil, buf); err != nil {
		return nil, fmt.Errorf("%s: read: %w", desc, err)
	}
	return parseWords(buf, nWords, desc)
}

func parseWords(buf []byte, nWords int, desc string) ([]uint16, error) {
	words := make([]uint16, nWords)
	for i := 0; i < nWords; i++ {
		off := i * 3
		data := buf[off : off+2]
		if crc8(data) != buf[off+2] {
			return nil, fmt.Errorf("%s: CRC mismatch at word %d", desc, i)
		}
		words[i] = binary.BigEndian.Uint16(data)
	}
	return words, nil
}

func crc8(data []byte) byte {
	crc := Crc8Init
	for _, b := range data {
		crc ^= b
		for bit := 0; bit < 8; bit++ {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ Crc8Polynomial
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func ticksToCelsius(ticks uint16) float64 {
	return -45 + 175*float64(ticks)/signalScale
}

func ticksToHumidity(ticks uint16) float64 {
	return 100 * float64(ticks) / signalScale
}

func offsetTicksToCelsius(ticks uint16) float64 {
	return 175 * float64(ticks) / signalScale
}

func celsiusToOffsetTicks(celsius float64) uint16 {
	if celsius < 0 {
		celsius = 0
	}
	v := celsius * signalScale / 175
	if v > signalScale {
		v = signalScale
	}
	return uint16(v + 0.5)
}

func celsiusToFahrenheit(celsius float64) float64 {
	return 1.8*celsius + 32
}
