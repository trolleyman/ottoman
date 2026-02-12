package input

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
	return nil
}

func (m *SimulatedMouse) LeftClick() error {
	return nil
}

func (m *SimulatedMouse) LeftDown() error {
	return nil
}

func (m *SimulatedMouse) LeftUp() error {
	return nil
}

func (m *SimulatedMouse) Type(text string) error {
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
