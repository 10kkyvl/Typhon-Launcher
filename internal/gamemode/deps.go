package gamemode

import (
	"time"

	"typhon/internal/procs"
)

func SystemDeps() Deps {
	return Deps{
		Power:    systemPowerImpl(),
		Priority: systemPriorityImpl(),
		Descendants: func(root uint32) ([]uint32, error) {
			table, err := procs.ParentsNow()
			if err != nil {
				return nil, err
			}
			return procs.Descendants(table, root), nil
		},
		After: time.After,
	}
}
