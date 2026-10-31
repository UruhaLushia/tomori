package launcher

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/serverstatus"
	"github.com/UruhaLushia/tomori/locales"
)

type LaunchConfig struct {
	Version         *minecraft.VersionDetail
	RootDir         string
	LibrariesDir    string
	ClientJar       string
	GameDir         string
	InstanceDir     string
	InstanceName    string
	AssetsDir       string
	GameAssetsDir   string
	NativesDir      string
	JavaPath        string
	Account         account.Account
	AuthlibPath     string
	AuthlibMetadata string
	MemoryMiB       int
	Width           int
	Height          int
	FullScreen      bool
	ProcessPriority config.ProcessPriority
	ExtraJVM        []string
	LauncherName    string
	LauncherVer     string
	Env             minecraft.Environment
	LogConfig       string
	Server          ServerTarget
	Execution       config.Execution
	Progress        func(minecraft.Progress)
}

func (c LaunchConfig) librariesDir() string {
	if c.LibrariesDir != "" {
		return c.LibrariesDir
	}
	return filepath.Join(c.RootDir, "libraries")
}

func (c LaunchConfig) clientJar() string {
	if c.ClientJar != "" {
		return c.ClientJar
	}
	id := c.Version.ClientID()
	return filepath.Join(c.RootDir, "versions", id, id+".jar")
}

func (c LaunchConfig) classpath() (string, error) {
	parts := make([]string, 0, len(c.Version.Libraries)+1)
	seen := make(map[string]bool)
	for _, l := range c.Version.LibrariesFor(c.Env) {
		a, err := l.Artifact()
		if err != nil {
			return "", err
		}
		if a != nil {
			path, err := fsutil.SafePath(c.librariesDir(), a.Path)
			if err != nil {
				return "", err
			}
			if seen[path] {
				continue
			}
			seen[path] = true
			parts = append(parts, path)
		}
	}
	parts = append(parts, c.clientJar())
	for _, path := range parts {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf(locales.MissingLaunchFile(), path, err)
		}
		if info.IsDir() || info.Size() == 0 {
			return "", fmt.Errorf(locales.InvalidLaunchFileAlt(), path)
		}
	}
	return strings.Join(parts, string(os.PathListSeparator)), nil
}

func (c LaunchConfig) Command() ([]string, error) {
	if c.Version == nil {
		return nil, errors.New(locales.NoVersionInformation())
	}
	if err := c.Version.Validate(); err != nil {
		return nil, err
	}
	if c.JavaPath == "" {
		return nil, errors.New(locales.NoJavaPath())
	}
	if err := c.Account.Validate(); err != nil {
		return nil, err
	}
	if c.Account.Kind == account.Microsoft && (c.Account.AccessToken == "" || !c.Account.ExpiresAt.After(time.Now())) {
		return nil, errors.New(locales.AccountExpiredSignInAgain())
	}
	env := c.Env
	env.Features = maps.Clone(env.Features)
	if env.Features == nil {
		env.Features = map[string]bool{}
	}
	if c.Width > 0 && c.Height > 0 {
		env.Features["has_custom_resolution"] = true
	}
	if c.Server.Address != "" {
		address, err := serverstatus.ParseAddress(c.Server.Address)
		if err != nil {
			return nil, err
		}
		c.Server.Address = address.String()
		env.Features["is_quick_play_multiplayer"] = true
	}
	cp, err := c.classpath()
	if err != nil {
		return nil, err
	}
	assetID := c.Version.AssetIndex.ID
	if assetID == "" {
		assetID = c.Version.Assets
	}
	vars := map[string]string{
		"auth_player_name":     c.Account.Name,
		"auth_uuid":            strings.ReplaceAll(c.Account.UUID, "-", ""),
		"auth_access_token":    "0",
		"auth_session":         "-",
		"user_type":            "legacy",
		"user_properties":      "{}",
		"clientid":             "",
		"xuid":                 "",
		"auth_xuid":            "",
		"version_name":         c.Version.ID,
		"version_type":         c.Version.Type,
		"game_directory":       c.GameDir,
		"assets_root":          c.AssetsDir,
		"assets_index_name":    assetID,
		"natives_directory":    c.NativesDir,
		"launcher_name":        c.LauncherName,
		"launcher_version":     c.LauncherVer,
		"classpath":            cp,
		"classpath_separator":  string(os.PathListSeparator),
		"library_directory":    c.librariesDir(),
		"resolution_width":     strconv.Itoa(c.Width),
		"resolution_height":    strconv.Itoa(c.Height),
		"game_assets":          filepath.Join(c.AssetsDir, "virtual", assetID),
		"quickPlayMultiplayer": c.Server.Address,
	}
	if c.GameAssetsDir != "" {
		vars["game_assets"] = c.GameAssetsDir
	}
	if c.Account.Kind == account.Microsoft {
		vars["auth_access_token"] = c.Account.AccessToken
		vars["auth_session"] = "token:" + c.Account.AccessToken + ":" + c.Account.UUID
		vars["user_type"] = "msa"
		vars["xuid"], vars["auth_xuid"] = c.Account.XUID, c.Account.XUID
	}
	if c.Account.Kind == account.Yggdrasil {
		vars["auth_access_token"], vars["auth_session"] = c.Account.AccessToken, c.Account.AccessToken
		vars["user_type"] = "mojang"
		if c.Account.Properties != "" {
			vars["user_properties"] = c.Account.Properties
		}
	}

	var jvm []string
	if c.MemoryMiB > 0 {
		jvm = append(jvm, fmt.Sprintf("-Xmx%dM", c.MemoryMiB), fmt.Sprintf("-Xms%dM", min(c.MemoryMiB/2, 1024)))
	}
	if len(c.Version.Arguments.JVM) > 0 {
		for _, arg := range c.Version.Arguments.JVM {
			if !minecraft.Allowed(arg.Rules, env) {
				continue
			}
			for _, value := range arg.Value {
				value = strings.ReplaceAll(value, "${version_name}.jar", filepath.Base(c.clientJar()))
				jvm = append(jvm, expand(value, vars))
			}
		}
	} else {
		jvm = append(jvm,
			"-Djava.library.path="+c.NativesDir,
			"-cp", cp,
		)
	}
	if c.LogConfig != "" && c.Version.Logging != nil && c.Version.Logging.Client != nil {
		jvm = append(jvm, expand(c.Version.Logging.Client.Argument, map[string]string{"path": c.LogConfig}))
	}

	var game []string
	if len(c.Version.Arguments.Game) > 0 {
		for _, arg := range c.Version.Arguments.Game {
			if !minecraft.Allowed(arg.Rules, env) {
				continue
			}
			for _, value := range arg.Value {
				game = append(game, expand(value, vars))
			}
		}
	} else {
		legacy, err := SplitArguments(c.Version.MinecraftArguments)
		if err != nil {
			return nil, err
		}
		for _, value := range legacy {
			game = append(game, expand(value, vars))
		}
		if c.Width > 0 && c.Height > 0 && !slices.Contains(game, "--width") {
			game = append(game, "--width", strconv.Itoa(c.Width), "--height", strconv.Itoa(c.Height))
		}
	}
	// The launcher setting owns the startup mode, including inherited profiles.
	game = slices.DeleteFunc(game, func(arg string) bool {
		return arg == "--fullscreen" || strings.HasPrefix(arg, "--fullscreen=")
	})
	if c.FullScreen {
		game = append(game, "--fullscreen")
	}
	game, err = c.serverArguments(game)
	if err != nil {
		return nil, err
	}
	for _, group := range [][]string{jvm, game} {
		for _, arg := range group {
			if strings.Contains(arg, "${") {
				return nil, fmt.Errorf(locales.UnsupportedLaunchPlaceholder2(), c.Account.Redact(arg))
			}
		}
	}
	jvm = append(jvm, c.ExtraJVM...)
	if c.Account.Kind == account.Yggdrasil {
		args, err := c.authlibArguments()
		if err != nil {
			return nil, err
		}
		jvm = append(jvm, args...)
	}

	args := append(jvm, c.Version.MainClass)
	args = append(args, game...)
	return args, nil
}

func expand(s string, vars map[string]string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	for key, value := range vars {
		s = strings.ReplaceAll(s, "${"+key+"}", value)
	}
	return s
}
