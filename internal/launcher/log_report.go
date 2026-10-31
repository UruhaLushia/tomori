package launcher

import (
	"context"

	"github.com/UruhaLushia/tomori/internal/diagnostic"
	"github.com/UruhaLushia/tomori/internal/storage"
)

func (s *Service) ExportLogReport(ctx context.Context, directory string, report diagnostic.Report, logsOnly bool) (string, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	size := uint64(4096 + len(report.Source) + len(report.System))
	for _, record := range report.Records {
		size += uint64(len(record.Clock) + len(record.Level) + len(record.Source) + len(record.Message) + 32)
	}
	if !logsOnly {
		for _, finding := range report.Findings {
			size += uint64(len(finding.Title) + len(finding.Advice) + 256)
			for _, evidence := range finding.Evidence {
				size += uint64(len(evidence) + 16)
			}
		}
	}
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: directory, Bytes: 2 * size}})
	if err != nil {
		return "", err
	}
	if err := space.Check(); err != nil {
		return "", err
	}
	return diagnostic.Export(ctx, directory, report, logsOnly)
}
