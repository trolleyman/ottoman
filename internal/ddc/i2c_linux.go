//go:build linux

package ddc

import (
	stderrors "errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// This file implements the DDC/CI protocol directly over /dev/i2c-N, as an
// alternative to shelling out to the `ddcutil` CLI (see ddc_linux.go). Each
// ddcutil call spawns a fresh process that re-opens and re-probes the bus —
// tens of milliseconds of pure overhead before any I2C happens — which is what
// makes dragging the brightness slider laggy. Talking I2C ourselves reduces a
// write to a single ~1-3ms bus transaction with no process spawn.
//
// The protocol's own timing constraints remain: DDC/CI mandates a pause between
// operations so the monitor's firmware can keep up. Rather than a fixed sleep we
// adapt the spacing per bus (like ddcutil's dynamic sleep): start spec-safe,
// creep faster while operations succeed, and back off exponentially with retries
// when the panel starts erroring. Bus discovery is likewise direct — EDIDs are
// read over i2c (see edid_linux.go), so ddcutil is no longer needed at all on a
// setup like this; it remains only as a detection fallback.

// i2cSlaveForce is the <linux/i2c-dev.h> I2C_SLAVE_FORCE ioctl — sets the
// target slave address even if a kernel driver holds it. x/sys/unix doesn't
// export the I2C constants in this build, but the value is stable kernel ABI.
const i2cSlaveForce = 0x0706

const (
	// ddcAddr is the fixed DDC/CI i2c slave address (7-bit).
	ddcAddr = 0x37
	// ddcHostAddr is the host's source address used in message framing.
	ddcHostAddr = 0x51
	// ddcWriteAddr is the 8-bit write address (ddcAddr<<1); it seeds the
	// checksum even though the kernel, not us, puts it on the wire.
	ddcWriteAddr = ddcAddr << 1

	// Adaptive inter-operation spacing. We start at ddcSpacingInit (spec-safe),
	// shave ddcSpacingStep toward ddcSpacingMin after a run of clean operations,
	// and double toward ddcSpacingMax on any failure. 45ms is the well-worn safe
	// default; a happy panel settles near the floor, a fussy one near the ceiling.
	ddcSpacingInit  = 45 * time.Millisecond  // starting / default spacing
	ddcSpacingMin   = 25 * time.Millisecond  // floor once the panel proves reliable
	ddcSpacingMax   = 120 * time.Millisecond // ceiling when it's struggling
	ddcSpacingStep  = 5 * time.Millisecond   // shaved off per speedup
	ddcSpeedupAfter = 8                      // consecutive OKs before speeding up
	ddcMaxAttempts  = 4                      // tries for one op before giving up

	// ddcReadDelay is how long to wait after sending a getvcp request before
	// reading the reply, per the DDC/CI spec.
	ddcReadDelay = 40 * time.Millisecond
)

// errUnsupportedFeature marks a clean "feature not supported" reply from the
// monitor (as opposed to a timing/I2C failure), so do() returns it immediately
// instead of retrying and backing off.
var errUnsupportedFeature = stderrors.New("DDC feature unsupported")

// i2cBus serializes DDC/CI operations on one bus and carries the adaptive
// spacing state: when the last op finished, the current spacing, and how many
// operations have succeeded in a row since the last speedup.
type i2cBus struct {
	bus     int
	mu      sync.Mutex
	lastOp  time.Time
	spacing time.Duration
	oks     int
}

var (
	busesMu sync.Mutex
	buses   = map[int]*i2cBus{}
)

func getBus(bus int) *i2cBus {
	busesMu.Lock()
	defer busesMu.Unlock()
	b := buses[bus]
	if b == nil {
		b = &i2cBus{bus: bus, spacing: ddcSpacingInit}
		buses[bus] = b
	}
	return b
}

// do runs fn against an open handle to the bus, holding the bus lock. It waits
// the current adaptive spacing before each attempt, and on a transient failure
// widens the spacing and retries; a permanent error (unsupported feature) is
// returned at once without backoff.
func (b *i2cBus) do(fn func(f *os.File) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var lastErr error
	for range ddcMaxAttempts {
		if wait := b.spacing - time.Since(b.lastOp); wait > 0 {
			time.Sleep(wait)
		}
		err := b.attempt(fn)
		b.lastOp = time.Now()

		if err == nil {
			b.faster()
			return nil
		}
		if stderrors.Is(err, errUnsupportedFeature) {
			return err
		}
		lastErr = err
		b.slower() // widen spacing; the next attempt waits longer
	}
	return errors.Wrapf(lastErr, "DDC op failed after %d attempts", ddcMaxAttempts)
}

// attempt opens the bus, targets the DDC/CI address, and runs one operation.
func (b *i2cBus) attempt(fn func(f *os.File) error) error {
	f, err := os.OpenFile(fmt.Sprintf("/dev/i2c-%d", b.bus), os.O_RDWR, 0)
	if err != nil {
		return errors.Wrapf(err, "open /dev/i2c-%d", b.bus)
	}
	defer f.Close()

	// I2C_SLAVE_FORCE (not I2C_SLAVE) because a stale kernel driver can hold the
	// address; 0x37 is DDC/CI-only so forcing is safe, and it's what ddcutil does.
	if err := unix.IoctlSetInt(int(f.Fd()), i2cSlaveForce, ddcAddr); err != nil {
		return errors.Wrap(err, "set i2c slave address")
	}
	return fn(f)
}

// faster shaves the spacing toward the floor after a run of clean operations.
func (b *i2cBus) faster() {
	b.oks++
	if b.oks >= ddcSpeedupAfter && b.spacing > ddcSpacingMin {
		b.spacing -= ddcSpacingStep
		if b.spacing < ddcSpacingMin {
			b.spacing = ddcSpacingMin
		}
		b.oks = 0
	}
}

// slower doubles the spacing toward the ceiling after a failure.
func (b *i2cBus) slower() {
	b.oks = 0
	b.spacing *= 2
	if b.spacing > ddcSpacingMax {
		b.spacing = ddcSpacingMax
	}
}

// ddcChecksum is the running XOR of the framing address and every payload byte.
func ddcChecksum(seed byte, data []byte) byte {
	c := seed
	for _, b := range data {
		c ^= b
	}
	return c
}

// setVCPMsg builds a "Set VCP Feature" command (op 0x03) for a 2-byte value,
// terminated by the DDC/CI checksum.
func setVCPMsg(code byte, value int) []byte {
	// source, length(0x80|4), set-vcp op, code, value-hi, value-lo
	payload := []byte{ddcHostAddr, 0x84, 0x03, code, byte(value >> 8), byte(value & 0xff)}
	return append(payload, ddcChecksum(ddcWriteAddr, payload))
}

// getVCPMsg builds a "Get VCP Feature" request (op 0x01) for a feature code.
func getVCPMsg(code byte) []byte {
	// source, length(0x80|2), get-vcp op, code
	req := []byte{ddcHostAddr, 0x82, 0x01, code}
	return append(req, ddcChecksum(ddcWriteAddr, req))
}

// setVCP writes a "Set VCP Feature" command (op 0x03) for a 2-byte value.
func setVCP(f *os.File, code byte, value int) error {
	if _, err := f.Write(setVCPMsg(code, value)); err != nil {
		return errors.Wrap(err, "write setvcp")
	}
	return nil
}

// getVCP writes a "Get VCP Feature" request (op 0x01) and parses the reply,
// returning the current and maximum raw values.
func getVCP(f *os.File, code byte) (cur, max int, err error) {
	if _, err := f.Write(getVCPMsg(code)); err != nil {
		return 0, 0, errors.Wrap(err, "write getvcp request")
	}

	time.Sleep(ddcReadDelay)

	// Reply: src(0x6E), len(0x88), reply-op(0x02), result, code, type,
	//        max-hi, max-lo, cur-hi, cur-lo, checksum = 11 bytes.
	buf := make([]byte, 11)
	n, err := f.Read(buf)
	if err != nil {
		return 0, 0, errors.Wrap(err, "read getvcp reply")
	}
	if n < 11 {
		return 0, 0, errors.Errorf("short DDC reply (%d bytes)", n)
	}
	if buf[2] != 0x02 {
		return 0, 0, errors.Errorf("unexpected DDC reply op 0x%02x", buf[2])
	}
	if buf[3] != 0x00 {
		return 0, 0, errors.Wrapf(errUnsupportedFeature, "feature 0x%02x (result 0x%02x)", code, buf[3])
	}
	max = int(buf[6])<<8 | int(buf[7])
	cur = int(buf[8])<<8 | int(buf[9])
	return cur, max, nil
}

// GetBrightnessDirect returns brightness (0-100) read over raw I2C.
func GetBrightnessDirect(bus int) (int, error) {
	var pct int
	err := getBus(bus).do(func(f *os.File) error {
		cur, max, err := getVCP(f, vcpBrightnessCode)
		if err != nil {
			return err
		}
		if max <= 0 {
			max = 100
		}
		pct = cur * 100 / max
		return nil
	})
	return pct, err
}

// SetBrightnessDirect sets brightness (0-100) over raw I2C.
func SetBrightnessDirect(bus, percent int) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return getBus(bus).do(func(f *os.File) error {
		return setVCP(f, vcpBrightnessCode, percent)
	})
}

// SetPowerDirect turns the display on (0xD6=1) or to standby (0xD6=4) over raw I2C.
func SetPowerDirect(bus int, on bool) error {
	value := powerStandby
	if on {
		value = powerOn
	}
	return getBus(bus).do(func(f *os.File) error {
		return setVCP(f, vcpPowerCode, value)
	})
}
