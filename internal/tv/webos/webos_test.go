package webos

import (
	"encoding/json"
	"testing"
)

func TestParseVolumeFlat(t *testing.T) {
	st, err := parseVolume(json.RawMessage(`{"volume":15,"muted":true,"returnValue":true}`))
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if st.Volume != 15 || !st.Muted {
		t.Fatalf("got %+v, want {15 true}", st)
	}
}

func TestParseVolumeNested(t *testing.T) {
	st, err := parseVolume(json.RawMessage(`{"volumeStatus":{"volume":22,"muteStatus":false}}`))
	if err != nil {
		t.Fatalf("parseVolume: %v", err)
	}
	if st.Volume != 22 || st.Muted {
		t.Fatalf("got %+v, want {22 false}", st)
	}
}

func TestBuildMagicPacket(t *testing.T) {
	packet, err := buildMagicPacket("8C:19:B5:72:FE:62")
	if err != nil {
		t.Fatalf("buildMagicPacket: %v", err)
	}
	if len(packet) != 102 {
		t.Fatalf("len = %d, want 102", len(packet))
	}
	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			t.Fatalf("byte %d = %#x, want 0xFF", i, packet[i])
		}
	}
	// First repeated MAC byte.
	if packet[6] != 0x8C {
		t.Fatalf("packet[6] = %#x, want 0x8C", packet[6])
	}

	if _, err := buildMagicPacket("nope"); err == nil {
		t.Fatal("expected error for bad MAC")
	}
}

func TestRegistrationPayloadIncludesKey(t *testing.T) {
	raw := registrationPayload("mykey123")
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["client-key"] != "mykey123" {
		t.Fatalf("client-key = %v, want mykey123", m["client-key"])
	}
	if _, ok := m["manifest"]; !ok {
		t.Fatal("manifest missing")
	}

	// No key -> field omitted.
	raw2 := registrationPayload("")
	var m2 map[string]any
	_ = json.Unmarshal(raw2, &m2)
	if _, ok := m2["client-key"]; ok {
		t.Fatal("client-key should be absent when empty")
	}
}
