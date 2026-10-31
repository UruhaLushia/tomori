package launcher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) prepareYggdrasil(ctx context.Context, cfg *LaunchConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client := account.NewClient(s.cfg.Download.Proxy, s.client.Budget)
	defer client.Close()
	var agent string
	var server account.YggdrasilServer
	var agentErr, metadataErr error
	var jobs sync.WaitGroup
	jobs.Go(func() {
		agent, agentErr = client.Authlib(ctx, s.cacheDir, minecraft.ParseSource(s.cfg.Download.Source) == minecraft.SourceMirror)
		if agentErr != nil {
			cancel()
		}
	})
	jobs.Go(func() {
		server, metadataErr = client.YggdrasilServer(ctx, cfg.Account.ServerURL, false)
		if metadataErr != nil {
			cancel()
		}
	})
	jobs.Wait()
	if agentErr != nil && !errors.Is(agentErr, context.Canceled) {
		return agentErr
	}
	if metadataErr != nil {
		return metadataErr
	}
	if agentErr != nil {
		return agentErr
	}
	cfg.AuthlibPath, cfg.AuthlibMetadata = agent, server.Metadata
	return nil
}

func (c LaunchConfig) authlibArguments() ([]string, error) {
	if !filepath.IsAbs(c.AuthlibPath) || strings.ContainsAny(c.AuthlibPath, "=\x00\r\n") || len(c.AuthlibMetadata) > 64<<10 || !json.Valid([]byte(c.AuthlibMetadata)) {
		return nil, errors.New(locales.AuthenticationAgentIsNotReady())
	}
	f, err := fsutil.OpenRegular(c.AuthlibPath)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	f.Close()
	if err != nil {
		return nil, err
	}
	if info.Size() <= 0 || info.Size() > 16<<20 {
		return nil, errors.New(locales.AuthenticationAgentIsNotReady())
	}
	return []string{"-javaagent:" + c.AuthlibPath + "=" + c.Account.ServerURL, "-Dauthlibinjector.yggdrasil.prefetched=" + base64.StdEncoding.EncodeToString([]byte(c.AuthlibMetadata))}, nil
}
