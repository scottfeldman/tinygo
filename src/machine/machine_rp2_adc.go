//go:build rp2040 || rp2350

package machine

import (
	"device/rp"
	"errors"
	"sync"
)

// ADCChannel is the ADC peripheral mux channel. 0-4.
type ADCChannel uint8

// Used to serialise ADC sampling
var adcLock sync.Mutex

// ADC peripheral reference voltage (mV)
var adcAref uint32

// InitADC resets the ADC peripheral.
func InitADC() {
	rp.RESETS.RESET.SetBits(rp.RESETS_RESET_ADC)
	rp.RESETS.RESET.ClearBits(rp.RESETS_RESET_ADC)
	for !rp.RESETS.RESET_DONE.HasBits(rp.RESETS_RESET_ADC) {
	}
	// enable ADC
	rp.ADC.CS.Set(rp.ADC_CS_EN)
	adcAref = 3300
	waitForReady()
}

// Configure sets the ADC pin to analog input mode.
func (a ADC) Configure(config ADCConfig) error {
	c, err := a.GetADCChannel()
	if err != nil {
		return err
	}
	return c.Configure(config)
}

// Get returns a one-shot ADC sample reading.
func (a ADC) Get() uint16 {
	if c, err := a.GetADCChannel(); err == nil {
		return c.getOnce()
	}
	// Not an ADC pin!
	return 0
}

// GetADCChannel returns the channel associated with the ADC pin.
func (a ADC) GetADCChannel() (c ADCChannel, err error) {
	if a.Pin < ADC0 {
		return 0, errors.New("no ADC channel for pin value")
	}
	return ADCChannel(a.Pin - ADC0), nil
}

// Configure sets the channel's associated pin to analog input mode.
// The powered on temperature sensor increases ADC_AVDD current by approximately 40 μA.
func (c ADCChannel) Configure(config ADCConfig) error {
	if config.Reference != 0 {
		adcAref = config.Reference
	}
	p, err := c.Pin()
	if err != nil {
		return err
	}
	p.Configure(PinConfig{Mode: PinAnalog})
	return nil
}

// getOnce returns a one-shot ADC sample reading from an ADC channel.
func (c ADCChannel) getOnce() uint16 {
	// Make it safe to sample multiple ADC channels in separate go routines.
	adcLock.Lock()
	rp.ADC.CS.ReplaceBits(uint32(c), 0b111, rp.ADC_CS_AINSEL_Pos)
	rp.ADC.CS.SetBits(rp.ADC_CS_START_ONCE)

	waitForReady()
	adcLock.Unlock()

	// rp2040 is a 12-bit ADC, scale raw reading to 16-bits.
	return uint16(rp.ADC.RESULT.Get()) << 4
}

// getVoltage does a one-shot sample and returns a millivolts reading.
// Integer portion is stored in the high 16 bits and fractional in the low 16 bits.
func (c ADCChannel) getVoltage() uint32 {
	return (adcAref << 16) / (1 << 12) * uint32(c.getOnce()>>4)
}

// ReadTemperature does a one-shot sample of the internal temperature sensor and returns a milli-celsius reading.
func ReadTemperature() (millicelsius int32) {
	if rp.ADC.CS.Get()&rp.ADC_CS_EN == 0 {
		InitADC()
	}
	thermChan, _ := ADC{Pin: thermADC}.GetADCChannel()
	// Enable temperature sensor bias source
	rp.ADC.CS.SetBits(rp.ADC_CS_TS_EN)

	// T = 27 - (ADC_voltage - 0.706)/0.001721
	return (27000<<16 - (int32(thermChan.getVoltage())-706<<16)*581) >> 16
}

// waitForReady spins waiting for the ADC peripheral to become ready.
func waitForReady() {
	for !rp.ADC.CS.HasBits(rp.ADC_CS_READY) {
	}
}

// The Pin method returns the GPIO Pin associated with the ADC mux channel, if it has one.
func (c ADCChannel) Pin() (p Pin, err error) {
	return Pin(c) + ADC0, nil
}
