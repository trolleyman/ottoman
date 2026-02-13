package input

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// MouseButton represents a mouse button.
type MouseButton int

const (
	MouseButtonLeft    MouseButton = 1
	MouseButtonMiddle  MouseButton = 2
	MouseButtonRight   MouseButton = 3
	MouseButtonBack    MouseButton = 4
	MouseButtonForward MouseButton = 5
)

// String returns the lowercase name of the mouse button.
func (b MouseButton) String() string {
	switch b {
	case MouseButtonLeft:
		return "left"
	case MouseButtonMiddle:
		return "middle"
	case MouseButtonRight:
		return "right"
	case MouseButtonBack:
		return "back"
	case MouseButtonForward:
		return "forward"
	default:
		return fmt.Sprintf("button(%d)", int(b))
	}
}

// ParseMouseButton parses a mouse button string. Returns MouseButtonLeft if empty or unrecognized.
func ParseMouseButton(s string) MouseButton {
	switch strings.ToLower(s) {
	case "left", "1", "":
		return MouseButtonLeft
	case "middle", "2":
		return MouseButtonMiddle
	case "right", "3":
		return MouseButtonRight
	case "back", "4":
		return MouseButtonBack
	case "forward", "5":
		return MouseButtonForward
	default:
		return MouseButtonLeft
	}
}

// MouseController provides platform-specific cursor and mouse button control.
type MouseController interface {
	// MoveTo sets the cursor to an absolute position.
	MoveTo(x, y int) error
	// GetPosition returns the current cursor position.
	GetPosition() (x, y int, err error)
	// MoveRelative moves the cursor by a delta, accumulating sub-pixel fractions.
	MoveRelative(dx, dy float64) error
	// Click performs a click of the given mouse button.
	Click(btn MouseButton) error
	// ButtonDown presses the given mouse button.
	ButtonDown(btn MouseButton) error
	// ButtonUp releases the given mouse button.
	ButtonUp(btn MouseButton) error
	// Scroll scrolls by dx (horizontal) and dy (vertical).
	// Positive dy = scroll down, negative dy = scroll up.
	// Positive dx = scroll right, negative dx = scroll left.
	// If precise is true, values are pixel-precise (trackpads); otherwise line-based (mouse wheels).
	Scroll(dx, dy int, precise bool) error
}

// KeyboardController provides platform-specific keyboard input.
type KeyboardController interface {
	// Type types the given text as Unicode characters.
	Type(text string) error
	// KeyPress sends a special key press with optional modifiers.
	// key is the browser event.key name (e.g. "ArrowLeft", "Enter", "F1").
	// modifiers is a list of modifier names: "shift", "ctrl", "alt", "meta".
	KeyPress(key string, modifiers []string) error
	// KeyDown presses a key.
	KeyDown(key string) error
	// KeyUp releases a key.
	KeyUp(key string) error
}

const (
	velocityBufferSize = 5
	inertiaTick        = 16 * time.Millisecond // ~60fps
	inertiaThreshold   = 0.5                   // stop when velocity magnitude below this
)

// InertiaEngine wraps a MouseController and provides velocity-based inertia for touch input.
type InertiaEngine struct {
	mu          sync.Mutex
	mouse       MouseController
	sensitivity float64
	friction    float64

	touchMode bool
	velXBuf   [velocityBufferSize]float64
	velYBuf   [velocityBufferSize]float64
	velIdx    int
	velCount  int

	cancelFunc context.CancelFunc

	// OnPosition is called after cursor moves with the new position.
	// May be called from any goroutine.
	OnPosition func(x, y int)
}

// NewInertiaEngine creates an inertia engine wrapping the given mouse controller.
func NewInertiaEngine(mouse MouseController, sensitivity, friction float64, onPosition func(x, y int)) *InertiaEngine {
	return &InertiaEngine{
		mouse:       mouse,
		sensitivity: sensitivity,
		friction:    friction,
		OnPosition:  onPosition,
	}
}

// Start begins a new drag interaction. Cancels any running inertia.
func (e *InertiaEngine) Start(touchMode bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.cancelInertia()
	e.touchMode = touchMode
	e.velIdx = 0
	e.velCount = 0
}

// Move applies a delta to the cursor and records velocity.
func (e *InertiaEngine) Move(dx, dy float64) {
	e.mu.Lock()
	sdx := dx * e.sensitivity
	sdy := dy * e.sensitivity

	// Record in velocity ring buffer
	e.velXBuf[e.velIdx] = sdx
	e.velYBuf[e.velIdx] = sdy
	e.velIdx = (e.velIdx + 1) % velocityBufferSize
	if e.velCount < velocityBufferSize {
		e.velCount++
	}

	mouse := e.mouse
	onPos := e.OnPosition
	e.mu.Unlock()

	mouse.MoveRelative(sdx, sdy)
	e.reportPosition(mouse, onPos)
}

// End finishes a drag interaction. If touch mode, starts inertia.
func (e *InertiaEngine) End() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.touchMode || e.velCount == 0 {
		return
	}

	// Compute average velocity from ring buffer
	var vx, vy float64
	n := e.velCount
	for i := 0; i < n; i++ {
		vx += e.velXBuf[i]
		vy += e.velYBuf[i]
	}
	vx /= float64(n)
	vy /= float64(n)

	if math.Abs(vx) < inertiaThreshold && math.Abs(vy) < inertiaThreshold {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFunc = cancel
	go e.runInertia(ctx, vx, vy)
}

func (e *InertiaEngine) cancelInertia() {
	if e.cancelFunc != nil {
		e.cancelFunc()
		e.cancelFunc = nil
	}
}

func (e *InertiaEngine) runInertia(ctx context.Context, vx, vy float64) {
	ticker := time.NewTicker(inertiaTick)
	defer ticker.Stop()

	friction := e.friction
	mouse := e.mouse
	onPos := e.OnPosition

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			vx *= friction
			vy *= friction

			if math.Abs(vx) > inertiaThreshold || math.Abs(vy) > inertiaThreshold {
				mouse.MoveRelative(vx, vy)
			}
			e.reportPosition(mouse, onPos)
		}
	}
}

func (e *InertiaEngine) reportPosition(mouse MouseController, onPos func(x, y int)) {
	if onPos == nil {
		return
	}
	x, y, err := mouse.GetPosition()
	if err != nil {
		return
	}
	onPos(x, y)
}
