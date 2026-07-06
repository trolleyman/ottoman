// Package ddc controls external monitors over DDC/CI (via the ddcutil CLI):
// brightness (VCP 0x10) and power/standby (VCP 0xD6). It works over both HDMI
// and DisplayPort. Only implemented on Linux.
package ddc

// Display is a DDC/CI-capable monitor discovered by ddcutil.
type Display struct {
	Bus    int    // i2c bus number (/dev/i2c-N)
	Mfg    string // manufacturer PNP id, e.g. "GSM"
	Model  string // model name, e.g. "LG ULTRAGEAR"
	Serial string // serial number (may be empty)
}

// VCP feature codes, as strings for the ddcutil CLI.
const (
	vcpBrightness = "10"
	vcpPower      = "D6"

	powerOn      = 1 // 0xD6 value 1 = on
	powerStandby = 4 // 0xD6 value 4 = standby/off
)

// VCP feature codes as raw bytes, for the direct-I2C transport.
const (
	vcpBrightnessCode = 0x10
	vcpPowerCode      = 0xD6
)
