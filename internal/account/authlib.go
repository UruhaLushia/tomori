package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

const authlibLimit = 16 << 20

type authlibArtifact struct {
	Build     int    `json:"build_number"`
	URL       string `json:"download_url"`
	Checksums struct {
		SHA256 string `json:"sha256"`
	} `json:"checksums"`
}

func (a authlibArtifact) valid() bool {
	u, err := url.Parse(a.URL)
	sum, e := hex.DecodeString(a.Checksums.SHA256)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && a.Build > 0 && e == nil && len(sum) == 32 && len(a.URL) < 4096
}

func (c *Client) filePermit(ctx context.Context) (func(), error) {
	if c.budget == nil {
		return func() {}, ctx.Err()
	}
	return c.budget.FilePermit(ctx)
}

func (c *Client) Authlib(ctx context.Context, cacheDir string, mirror bool) (string, error) {
	directory := filepath.Join(cacheDir, download.AuthlibCache)
	if c.budget != nil {
		release, err := c.budget.Lock(ctx, "authlib", directory)
		if err != nil {
			return "", err
		}
		defer release()
	}
	old, fresh := c.cachedAuthlib(ctx, directory)
	if fresh {
		return filepath.Join(directory, old.Checksums.SHA256+".jar"), nil
	}
	endpoint := "https://authlib-injector.yushi.moe/artifact/latest.json"
	if mirror {
		endpoint = "https://bmclapi2.bangbang93.com/mirrors/authlib-injector/artifact/latest.json"
	}
	var artifact authlibArtifact
	if err := c.request(ctx, "GET", endpoint, "", nil, &artifact); err != nil {
		if old.valid() && ctx.Err() == nil {
			return filepath.Join(directory, old.Checksums.SHA256+".jar"), nil
		}
		return "", err
	}
	if !artifact.valid() {
		return "", errors.New(locales.InvalidAuthenticationAgentMetadata())
	}
	artifact.Checksums.SHA256 = strings.ToLower(artifact.Checksums.SHA256)
	name := artifact.Checksums.SHA256 + ".jar"
	release, err := c.filePermit(ctx)
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(directory, 0700)
	var root *os.Root
	if err == nil {
		root, err = os.OpenRoot(directory)
	}
	release()
	if err != nil {
		return "", err
	}
	defer root.Close()
	release, err = c.filePermit(ctx)
	if err != nil {
		return "", err
	}
	valid := validAuthlib(ctx, root, name, artifact.Checksums.SHA256)
	release()
	if !valid {
		if err := c.downloadAuthlib(ctx, root, directory, name, artifact); err != nil {
			return "", err
		}
	}
	release, err = c.filePermit(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	raw, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	if err := fsutil.WriteRootAtomic(ctx, root, "artifact.json", raw); err != nil {
		return "", err
	}
	return filepath.Join(directory, name), nil
}

func (c *Client) cachedAuthlib(ctx context.Context, directory string) (authlibArtifact, bool) {
	var artifact authlibArtifact
	release, err := c.filePermit(ctx)
	if err != nil {
		return artifact, false
	}
	defer release()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return artifact, false
	}
	defer root.Close()
	raw, err := fsutil.ReadRootFile(root, "artifact.json", 16<<10)
	if err != nil || json.Unmarshal(raw, &artifact) != nil || !artifact.valid() {
		return authlibArtifact{}, false
	}
	if !validAuthlib(ctx, root, artifact.Checksums.SHA256+".jar", artifact.Checksums.SHA256) {
		return authlibArtifact{}, false
	}
	info, err := root.Stat("artifact.json")
	return artifact, err == nil && time.Since(info.ModTime()) < 24*time.Hour
}

func validAuthlib(ctx context.Context, root *os.Root, name, digest string) bool {
	f, err := fsutil.OpenRootRegular(root, name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() <= 0 || info.Size() > authlibLimit {
		return false
	}
	h := sha256.New()
	n, err := io.Copy(h, fsutil.ContextReader(ctx, io.LimitReader(f, authlibLimit+1)))
	return err == nil && n == info.Size() && hex.EncodeToString(h.Sum(nil)) == digest
}

func (c *Client) downloadAuthlib(ctx context.Context, root *os.Root, directory, name string, artifact authlibArtifact) error {
	report, err := storage.Assess(ctx, []storage.Requirement{{Directory: directory, Bytes: authlibLimit}})
	if err != nil {
		return err
	}
	if err := report.Check(); err != nil {
		return err
	}
	release, err := c.filePermit(ctx)
	if err != nil {
		return err
	}
	defer release()
	response, networkRelease, err := c.publicAuthGet(ctx, artifact.URL, true)
	if err != nil {
		return err
	}
	defer networkRelease()
	defer response.Body.Close()
	if response.ContentLength > authlibLimit {
		return errors.New(locales.AuthenticationAgentIsTooLarge())
	}
	stage := ".tomori-authlib-" + rand.Text()
	out, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(stage) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), fsutil.ContextReader(ctx, io.LimitReader(response.Body, authlibLimit+1)))
	if err == nil && (n <= 0 || n > authlibLimit || hex.EncodeToString(h.Sum(nil)) != artifact.Checksums.SHA256) {
		err = errors.New(locales.AuthenticationAgentVerificationFailed())
	}
	if err == nil {
		err = out.Sync()
	}
	if e := out.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return root.Rename(stage, name)
}
