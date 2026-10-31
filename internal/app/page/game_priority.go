package page

import (
	"runtime"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

var processPriorities = []struct {
	value config.ProcessPriority
	label func() string
}{
	{config.PriorityHighest, locales.Highest},
	{config.PriorityHigh, locales.High},
	{config.PriorityAboveNormal, locales.AboveNormal},
	{config.PriorityNormal, locales.Normal},
	{config.PriorityBelowNormal, locales.BelowNormal},
	{config.PriorityLow, locales.Low},
}

func priorityLabel(value config.ProcessPriority, label func() string) string {
	if value == config.PriorityHighest && runtime.GOOS == "windows" {
		return locales.Realtime()
	}
	return label()
}

func processPriorityInput(c *ui.Context, value *config.ProcessPriority, save func()) {
	options := []kit.SelectOption[config.ProcessPriority]{{Value: config.PriorityLow, Label: locales.Low()}, {Value: config.PriorityNormal, Label: locales.Normal()}, {Value: config.PriorityHigh, Label: locales.High()}}
	if *value != config.PriorityLow && *value != config.PriorityNormal && *value != config.PriorityHigh {
		for _, priority := range processPriorities {
			if priority.value == *value {
				options = append(options, kit.SelectOption[config.ProcessPriority]{Value: *value, Label: priorityLabel(*value, priority.label)})
				break
			}
		}
	}
	c.Key("process-priority")
	kit.SegmentedValues(c, value, options, save).Label(locales.ProcessPriority())
}
