package display

import (
	"slices"
	"sort"
	"strconv"

	"github.com/trolleyman/ottoman/internal/api"
)

func SortLayouts(layouts []api.Layout) {
	sort.Slice(layouts, func(i, j int) bool {
		a := layouts[i]
		b := layouts[j]

		getAliasNum := func(aliases []string) (int, bool) {
			for _, alias := range aliases {
				if num, err := strconv.Atoi(alias); err == nil {
					return num, true
				}
			}
			return 0, false
		}

		aNum, aOk := getAliasNum(a.Aliases)
		bNum, bOk := getAliasNum(b.Aliases)

		if aOk && bOk {
			if aNum != bNum {
				return aNum < bNum
			}
		}
		if a.Id != b.Id {
			return a.Id < b.Id
		}
		return false
	})
}

func SortMonitors(monitors []api.Monitor) {
	slices.SortFunc(monitors, func(a, b api.Monitor) int {
		if a.Active == nil && b.Active != nil {
			return 1
		}
		if a.Active != nil && b.Active == nil {
			return -1
		}
		if a.Active == nil && b.Active == nil {
			return slices.Compare([]rune(a.Edid), []rune(b.Edid))
		}
		ax := a.Active.PositionX
		bx := b.Active.PositionX
		if ax != bx {
			return ax - bx
		}
		return slices.Compare([]rune(a.Edid), []rune(b.Edid))
	})
}
