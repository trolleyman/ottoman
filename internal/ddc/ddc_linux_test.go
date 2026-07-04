//go:build linux

package ddc

import "testing"

const sampleDetect = `Display 1
   I2C bus:  /dev/i2c-4
   DRM connector:           card1-DP-1
   EDID synopsis:
      Mfg id:               GSM  Goldstar Company Ltd
      Model:                LG ULTRAGEAR
      Product code:         30140
      Serial number:        1234ABCD
      Binary serial number: 16843009 (0x01010101)
   VCP version:         2.1

Display 2
   I2C bus:  /dev/i2c-7
   DRM connector:           card1-HDMI-A-1
   EDID synopsis:
      Mfg id:               DEL
      Model:                DELL U2717D
      Serial number:
   VCP version:         2.1

Invalid display
   I2C bus:  /dev/i2c-9
   EDID synopsis:
      Mfg id:               XXX
      DDC communication failed
`

func TestParseDetect(t *testing.T) {
	displays := parseDetect(sampleDetect)
	// The "Invalid display" block is skipped (only "Display N" blocks count).
	if len(displays) != 2 {
		t.Fatalf("expected 2 valid displays, got %d: %+v", len(displays), displays)
	}
	for _, d := range displays {
		if d.Mfg == "XXX" {
			t.Fatal("invalid display leaked into results")
		}
	}

	if displays[0].Bus != 4 {
		t.Errorf("display[0].Bus = %d, want 4", displays[0].Bus)
	}
	if displays[0].Mfg != "GSM" {
		t.Errorf("display[0].Mfg = %q, want GSM", displays[0].Mfg)
	}
	if displays[0].Model != "LG ULTRAGEAR" {
		t.Errorf("display[0].Model = %q", displays[0].Model)
	}
	if displays[0].Serial != "1234ABCD" {
		t.Errorf("display[0].Serial = %q", displays[0].Serial)
	}

	if displays[1].Bus != 7 || displays[1].Mfg != "DEL" || displays[1].Model != "DELL U2717D" {
		t.Errorf("display[1] mismatch: %+v", displays[1])
	}
	if displays[1].Serial != "" {
		t.Errorf("display[1].Serial should be empty, got %q", displays[1].Serial)
	}
}

func TestParseVCPValue(t *testing.T) {
	out := "VCP code 0x10 (Brightness                    ): current value =    75, max value =   100\n"
	cur, max, err := parseVCPValue(out)
	if err != nil {
		t.Fatalf("parseVCPValue: %v", err)
	}
	if cur != 75 || max != 100 {
		t.Fatalf("got cur=%d max=%d, want 75/100", cur, max)
	}

	if _, _, err := parseVCPValue("nope"); err == nil {
		t.Error("expected error for unparseable VCP output")
	}
}
