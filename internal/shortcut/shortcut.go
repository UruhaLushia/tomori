package shortcut

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/launchrequest"
	"github.com/UruhaLushia/tomori/locales"
)

func Create(ctx context.Context, directory, name, executable, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ext, data, mode, err := content(runtime.GOOS, name, executable, key)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	stage := ".tomori-shortcut-" + rand.Text()
	out, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Remove(stage) }()
	_, err = out.Write(data)
	if err == nil {
		err = out.Chmod(mode)
	}
	if err == nil {
		err = out.Sync()
	}
	info, statErr := out.Stat()
	if err == nil {
		err = statErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	base := filename(name)
	for i := range 100 {
		name := base + ext
		if i > 0 {
			name = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		err := fsutil.PublishRootExclusive(ctx, root, stage, name, info)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err = root.Chmod(name, mode); err != nil {
			return "", err
		}
		return filepath.Join(directory, name), nil
	}
	return "", errors.New(locales.TooManyFilesWithTheSameName())
}

func content(platform, name, executable, key string) (string, []byte, os.FileMode, error) {
	link := launchrequest.Link(key)
	if _, err := launchrequest.ParseURL(link); err != nil {
		return "", nil, 0, err
	}
	if executable == "" || len(executable) > 8192 || !utf8.ValidString(executable) || strings.IndexFunc(executable, unicode.IsControl) >= 0 {
		return "", nil, 0, errors.New(locales.InvalidLauncherPath())
	}
	switch platform {
	case "linux":
		var title strings.Builder
		for _, r := range name {
			if title.Len()+utf8.RuneLen(r) > 256 {
				break
			}
			if !unicode.IsControl(r) {
				title.WriteRune(r)
			}
		}
		name = title.String()
		name = strings.ReplaceAll(name, `\`, `\\`)
		command := execArg(executable)
		if strings.ContainsRune(executable, '%') {
			command = execArg("/bin/sh") + " -c " + execArg(`exec "$@"`) + " tomori " + command
		}
		data := "[Desktop Entry]\nType=Application\nName=Tomori · " + name + "\nExec=" + command + " launch " + execArg(key) + "\nIcon=tomori\nTerminal=false\nStartupNotify=false\nCategories=Game;\n"
		return ".desktop", []byte(data), 0700, nil
	case "windows":
		text := "[InternetShortcut]\r\nURL=" + link + "\r\nIconFile=" + executable + "\r\nIconIndex=0\r\n"
		units := utf16.Encode([]rune(text))
		data := make([]byte, 2+len(units)*2)
		binary.LittleEndian.PutUint16(data, 0xfeff)
		for i, value := range units {
			binary.LittleEndian.PutUint16(data[2+i*2:], value)
		}
		return ".url", data, 0600, nil
	case "darwin":
		var data bytes.Buffer
		data.WriteString(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>URL</key><string>`)
		if err := xml.EscapeText(&data, []byte(link)); err != nil {
			return "", nil, 0, err
		}
		data.WriteString("</string></dict></plist>\n")
		return ".webloc", data.Bytes(), 0600, nil
	default:
		return "", nil, 0, errors.New(locales.CreatingShortcutsIsNotSupportedOnThisPlatform())
	}
}

func execArg(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '`', '$':
			out.WriteString(`\\`)
			out.WriteRune(r)
		case '\\':
			out.WriteString(`\\\\`)
		case '%':
			out.WriteString("%%")
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String()
}

func filename(name string) string {
	var out strings.Builder
	out.WriteString("Tomori")
	if name != "" {
		out.WriteString(" - ")
	}
	for _, r := range name {
		if out.Len()+utf8.RuneLen(r) > 120 {
			break
		}
		if unicode.IsControl(r) || strings.ContainsRune(`/\:<>"|?*`, r) {
			r = '-'
		}
		out.WriteRune(r)
	}
	return strings.TrimRight(out.String(), " .")
}
