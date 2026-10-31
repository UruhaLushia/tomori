package launcher

import (
	"context"
	"fmt"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/systeminfo"
	"github.com/UruhaLushia/tomori/locales"
)

func launchMemory(ctx context.Context, game config.Game, arch string) (int, error) {
	memory, err := systeminfo.ReadMemory(ctx)
	if err != nil {
		return 0, fmt.Errorf(locales.MemoryReadFailedWithError(), err)
	}
	value := game.MemoryMiB
	if game.MemoryAuto {
		value = memory.SuggestedMiB()
	}
	if value <= 0 || value > memory.TotalMiB {
		return 0, fmt.Errorf(locales.GameMemoryMustBeBetween1AndMiB(), memory.TotalMiB)
	}
	if arch == "x86" || arch == "i386" || arch == "i686" {
		if game.MemoryAuto {
			value = min(value, 1536)
		} else if value > 1536 {
			return 0, fmt.Errorf(locales.Value32BitJavaCannotAllocateMiBChoose64BitJava(), value)
		}
	}
	return value, nil
}
