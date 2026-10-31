package launchrequest

import (
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type Action uint8

const (
	None Action = iota
	Launch
	Open
)

type Request struct {
	Action    Action
	Instance  string
	Directory string
	URL       bool
}

func Parse(args []string, directory string) (Request, error) {
	invalid := func() (Request, error) { return Request{}, errors.New(locales.InvalidLaunchArguments()) }
	if len(args) == 1 && strings.HasPrefix(strings.ToLower(args[0]), "tomori:") {
		return ParseURL(args[0])
	}
	if len(args) != 2 {
		return invalid()
	}
	var action Action
	switch args[0] {
	case "launch":
		action = Launch
	case "open":
		action = Open
	default:
		return invalid()
	}
	if !validSelector(args[1]) {
		return invalid()
	}
	return Request{Action: action, Instance: args[1], Directory: directory}, nil
}

func ParseURL(raw string) (Request, error) {
	invalid := func() (Request, error) { return Request{}, errors.New(locales.InvalidTomoriLink()) }
	if len(raw) > 16<<10 {
		return invalid()
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "tomori") || u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawFragment != "" || (u.Path != "" && u.Path != "/") {
		return invalid()
	}
	var action Action
	switch strings.ToLower(u.Host) {
	case "launch":
		action = Launch
	case "instance":
		action = Open
	default:
		return invalid()
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["instance"]) != 1 {
		return invalid()
	}
	key := query.Get("instance")
	if !validSelector(key) || !isKey(key) {
		return invalid()
	}
	return Request{Action: action, Instance: key, URL: true}, nil
}

func Link(key string) string {
	return "tomori://launch?" + url.Values{"instance": {key}}.Encode()
}

func validSelector(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func isKey(value string) bool {
	format, path, ok := strings.Cut(value, ":")
	return ok && path != "" && (format == string(minecraft.InstanceMinecraft) || format == string(minecraft.InstancePrism) || format == string(minecraft.InstanceManaged))
}

func (r Request) Resolve(instances []minecraft.Instance) (minecraft.Instance, error) {
	for _, instance := range instances {
		if instance.Key() == r.Instance || runtime.GOOS == "windows" && strings.EqualFold(instance.Key(), r.Instance) {
			return instance, nil
		}
	}
	if r.URL || isKey(r.Instance) {
		return minecraft.Instance{}, errors.New(locales.InstanceNotFound())
	}
	path := r.Instance
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.Directory, path)
	}
	path = filepath.Clean(path)
	var pathMatch minecraft.Instance
	pathFound := false
	for _, instance := range instances {
		equal := filepath.Clean(instance.InstanceDir) == path
		if runtime.GOOS == "windows" {
			equal = strings.EqualFold(filepath.Clean(instance.InstanceDir), path)
		}
		if equal {
			if pathFound {
				return minecraft.Instance{}, errors.New(locales.MultipleInstancesMatchUseAnInstanceKeyOrPath())
			}
			pathMatch, pathFound = instance, true
		}
	}
	if pathFound {
		return pathMatch, nil
	}
	var match minecraft.Instance
	found := false
	for _, instance := range instances {
		if instance.Name != r.Instance && instance.ID != r.Instance {
			continue
		}
		if found {
			return minecraft.Instance{}, errors.New(locales.MultipleInstancesMatchUseAnInstanceKeyOrPath())
		}
		match, found = instance, true
	}
	if !found {
		return match, errors.New(locales.InstanceNotFound())
	}
	return match, nil
}
