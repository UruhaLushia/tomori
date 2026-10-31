package page

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type SpaceData struct {
	Loading bool
	Error   string
	Report  storage.Report
}

func (s SpaceData) Blocked() bool { return s.Loading || s.Report.Check() != nil }

func SpaceSummary(c *ui.Context, data SpaceData) {
	if !data.Loading && data.Error == "" && len(data.Report.Volumes) == 0 {
		return
	}
	status := kit.StatusData{}
	switch {
	case data.Loading:
		status.Loading = locales.CheckingDiskSpace2()
	case data.Error != "":
		status.Hint = locales.SpaceEstimateUnavailable()
	default:
		for _, volume := range data.Report.Volumes {
			row := kit.StatusRow{Value: fmt.Sprintf(locales.EstimatedAvailable(), storage.Size(volume.Required), storage.Size(volume.Available))}
			if volume.Required > volume.Available {
				row.Kind = kit.FeedbackError
			}
			status.Rows = append(status.Rows, row)
		}
		if data.Report.Check() != nil {
			status.Error = locales.NotEnoughDiskSpace()
		}
	}
	kit.StatusSummary(c, status)
}
