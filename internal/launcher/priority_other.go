//go:build !linux && !darwin && !windows

package launcher

import (
	"errors"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/locales"
)

func setProcessPriority(_ int, priority config.ProcessPriority) error {
	if priority == config.PriorityNormal {
		return nil
	}
	return errors.New(locales.ProcessPriorityIsUnsupportedOnThisPlatform())
}
