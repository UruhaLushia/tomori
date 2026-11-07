package download

import (
	"fmt"
	"os"

	"github.com/UruhaLushia/tomori/internal/cache"
)

func (f *Client) segmentedPath(t Task) string {
	if f.CacheDir == "" {
		return t.Path + ".tomori-download"
	}
	identity := t.URL + "|" + t.Path + "|" + t.SHA1 + fmt.Sprintf("|%d", t.Size)
	if t.SHA512 != "" {
		identity += "|" + t.SHA512
	}
	return cache.File(f.CacheDir, identity)
}

func (f *Client) removeSegmentedFiles(t Task) {
	output := f.segmentedPath(t)
	for _, name := range []string{output, output + ".part", output + ".part.resume"} {
		_ = os.Remove(name)
	}
}
