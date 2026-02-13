package input

import "log"

// SimulatedMouse is an in-memory mouse controller for testing without OS calls.
type SimulatedMouse struct {
	x, y         int
	minX, minY   int
	maxX, maxY   int
	fracX, fracY float64
}

// NewSimulatedMouse creates a simulated mouse at (startX, startY) clamped to [minX, maxX] x [minY, maxY].
func NewSimulatedMouse(startX, startY, minX, minY, maxX, maxY int) *SimulatedMouse {
	return &SimulatedMouse{x: startX, y: startY, minX: minX, minY: minY, maxX: maxX, maxY: maxY}
}

func (m *SimulatedMouse) MoveTo(x, y int) error {
	m.x = clamp(x, m.minX, m.maxX)
	m.y = clamp(y, m.minY, m.maxY)
	log.Printf("[SIM] Mouse Move To (%d, %d)", m.x, m.y)
	return nil
}

func (m *SimulatedMouse) GetPosition() (int, int, error) {
	return m.x, m.y, nil
}

func (m *SimulatedMouse) MoveRelative(dx, dy float64) error {
	m.fracX += dx
	m.fracY += dy

	intX := int(m.fracX)
	intY := int(m.fracY)

	if intX != 0 || intY != 0 {
		m.fracX -= float64(intX)
		m.fracY -= float64(intY)
		m.x = clamp(m.x+intX, m.minX, m.maxX)
		m.y = clamp(m.y+intY, m.minY, m.maxY)
	}
	log.Printf("[SIM] Mouse Move (Relative by %.2f, %.2f) => (%d, %d)", dx, dy, m.x, m.y)
	return nil
}

func (m *SimulatedMouse) Click(btn MouseButton) error {
	log.Printf("[SIM] %s Click", btn)
	return nil
}

func (m *SimulatedMouse) ButtonDown(btn MouseButton) error {
	log.Printf("[SIM] %s Down", btn)
	return nil
}

func (m *SimulatedMouse) ButtonUp(btn MouseButton) error {
	log.Printf("[SIM] %s Up", btn)
	return nil
}

func (m *SimulatedMouse) Scroll(dx, dy int, precise bool) error {
	log.Printf("[SIM] Scroll dx=%d dy=%d precise=%v", dx, dy, precise)
	return nil
}

// SimulatedKeyboard is an in-memory keyboard controller for testing without OS calls.
type SimulatedKeyboard struct{}

// NewSimulatedKeyboard creates a simulated keyboard.
func NewSimulatedKeyboard() *SimulatedKeyboard {
	return &SimulatedKeyboard{}
}

func (k *SimulatedKeyboard) KeyDown(key string, modifiers []string) error {
	log.Printf("[SIM] KeyDown: %s mod=%v", key, modifiers)
	return nil
}

func (k *SimulatedKeyboard) KeyUp(key string, modifiers []string) error {
	log.Printf("[SIM] KeyUp: %s mod=%v", key, modifiers)
	return nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
