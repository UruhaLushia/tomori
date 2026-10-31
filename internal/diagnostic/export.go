package diagnostic

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func Export(ctx context.Context, directory string, report Report, logsOnly bool) (_ string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	suffix := ".txt"
	if logsOnly {
		suffix = ".log"
	}
	name := "tomori-report-" + time.Now().Format("20060102-150405") + "-" + rand.Text()[:8] + suffix
	temp := ".tomori-report-" + rand.Text()
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	defer func() {
		if current, e := root.Lstat(temp); e == nil && os.SameFile(info, current) {
			err = errors.Join(err, root.Remove(temp))
		}
	}()
	if err := Write(ctx, f, report, logsOnly); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	info, err = f.Stat()
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := fsutil.PublishRootExclusive(ctx, root, temp, name, info); err != nil {
		return "", err
	}
	return filepath.Join(directory, name), nil
}

func Write(ctx context.Context, writer io.Writer, report Report, logsOnly bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	out := bufio.NewWriterSize(&reportWriter{ctx: ctx, writer: writer, left: MaxBytes + (256 << 10)}, 32<<10)
	if !logsOnly {
		_, _ = fmt.Fprintln(out, locales.TomoriDiagnosticReport())
		_, _ = fmt.Fprintf(out, "%s: %s\n%s: %s\n%s: %s\n", locales.Time(), report.Created.Format(time.RFC3339), locales.Source3(), report.Source, locales.Environment(), report.System)
		if report.Truncated {
			_, _ = fmt.Fprintln(out, locales.RecentRecordsOnly())
		}
		_, _ = fmt.Fprintln(out, "\n"+locales.Diagnose())
		if len(report.Findings) == 0 {
			_, _ = fmt.Fprintln(out, locales.NoClearCauseIdentified())
		}
		for _, finding := range report.Findings {
			if err := ctx.Err(); err != nil {
				return err
			}
			label := locales.ReportedByLog()
			if finding.Possible {
				label = locales.PossibleCause()
			}
			_, _ = fmt.Fprintf(out, "\n[%s] %s\n%s\n", label, finding.Title, finding.Advice)
			for _, evidence := range finding.Evidence {
				_, _ = fmt.Fprintln(out, "  "+evidence)
			}
		}
		_, _ = fmt.Fprintln(out, "\n"+locales.Logs())
	}
	for _, record := range report.Records {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "%s [%s] %s · %s\n", record.Clock, record.Level, record.Source, strings.TrimRight(record.Message, "\r\n"))
	}
	return out.Flush()
}

type reportWriter struct {
	ctx    context.Context
	writer io.Writer
	left   int
}

func (w *reportWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if len(data) > w.left {
		return 0, errors.New(locales.DiagnosticReportExceedsSizeLimit())
	}
	n, err := w.writer.Write(data)
	w.left -= n
	return n, err
}
