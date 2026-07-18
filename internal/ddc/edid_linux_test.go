//go:build linux

package ddc

import "testing"

// buildEDID assembles a minimal valid EDID base block with the given packed
// manufacturer bytes, binary serial, and up to two display descriptors.
func buildEDID(mfgHi, mfgLo byte, binSerial uint32, descs map[byte]string) []byte {
	e := make([]byte, edidLen)
	copy(e, edidHeader)
	e[8], e[9] = mfgHi, mfgLo
	e[12] = byte(binSerial)
	e[13] = byte(binSerial >> 8)
	e[14] = byte(binSerial >> 16)
	e[15] = byte(binSerial >> 24)

	offsets := []int{54, 72, 90, 108}
	i := 0
	for tag, text := range descs {
		off := offsets[i]
		i++
		e[off+3] = tag // 00 00 00 <tag> 00 ...
		payload := append([]byte(text), 0x0A)
		copy(e[off+5:off+18], payload)
	}
	return e
}

func TestParseEDID(t *testing.T) {
	// AOC with a text serial descriptor (0xFF): serial comes from the text.
	aoc := buildEDID(0x05, 0xE3, 0, map[byte]string{
		0xFC: "Q27G4",
		0xFF: "2S6R6HA023228",
	})
	// Philips with no text serial: serial falls back to the binary number.
	phl := buildEDID(0x41, 0x0C, 0x00001910, map[byte]string{
		0xFC: "PHL 243V7",
	})

	cases := []struct {
		name               string
		raw                []byte
		mfg, model, serial string
	}{
		{"aoc-text-serial", aoc, "AOC", "Q27G4", "2S6R6HA023228"},
		{"phl-binary-serial", phl, "PHL", "PHL 243V7", "0x00001910"},
	}
	for _, c := range cases {
		d, ok := parseEDID(c.raw)
		if !ok {
			t.Fatalf("%s: parseEDID returned ok=false", c.name)
		}
		if d.Mfg != c.mfg || d.Model != c.model || d.Serial != c.serial {
			t.Errorf("%s: got %q:%q:%q, want %q:%q:%q",
				c.name, d.Mfg, d.Model, d.Serial, c.mfg, c.model, c.serial)
		}
	}
}

func TestParseEDIDRejectsGarbage(t *testing.T) {
	if _, ok := parseEDID(make([]byte, edidLen)); ok {
		t.Error("parseEDID accepted a block with no EDID header")
	}
	if _, ok := parseEDID([]byte{0x00, 0xFF}); ok {
		t.Error("parseEDID accepted a short block")
	}
}
