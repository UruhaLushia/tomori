package minecraft

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func operatingSystemVersion() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}
