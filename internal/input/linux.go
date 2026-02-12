//go:build linux

package input

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// LinuxMouse controls the OS cursor via xdotool.
type LinuxMouse struct {
	fracX, fracY float64
}

// NewMouseController creates a platform-specific mouse controller.
func NewMouseController() (MouseController, error) {
	if _, err := exec.LookPath("xdotool"); err != nil {
		return nil, errors.Wrap(err, "xdotool not found")
	}
	return &LinuxMouse{}, nil
}

func (m *LinuxMouse) MoveTo(x, y int) error {
	cmd := exec.Command("xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y))
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "xdotool mousemove failed: %s", string(out))
	}
	return nil
}

func (m *LinuxMouse) GetPosition() (int, int, error) {
	cmd := exec.Command("xdotool", "getmouselocation", "--shell")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, errors.Wrap(err, "xdotool getmouselocation failed")
	}

	var x, y int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "X="); ok {
			x, _ = strconv.Atoi(v)
		} else if v, ok := strings.CutPrefix(line, "Y="); ok {
			y, _ = strconv.Atoi(v)
		}
	}
	return x, y, nil
}

func (m *LinuxMouse) MoveRelative(dx, dy float64) error {
	m.fracX += dx
	m.fracY += dy

	intX := int(m.fracX)
	intY := int(m.fracY)

	if intX == 0 && intY == 0 {
		return nil
	}

	m.fracX -= float64(intX)
	m.fracY -= float64(intY)

	// xdotool mousemove_relative -- dx dy (-- needed for negative values)
	cmd := exec.Command("xdotool", "mousemove_relative", "--",
		fmt.Sprintf("%d", intX), fmt.Sprintf("%d", intY))
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "xdotool mousemove_relative failed: %s", string(out))
	}
	return nil
}
