package loader

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func (i *Installer) runProcessor(ctx context.Context, p processor, vars map[string]string, work string) error {
	if len(p.Sides) > 0 && !slices.Contains(p.Sides, "client") {
		return nil
	}
	outputs := make(map[string]string)
	for name, sum := range p.Outputs {
		dst, err := i.value(name, vars, work, 0)
		if err != nil {
			return err
		}
		hash, err := i.value(sum, vars, work, 0)
		if err != nil {
			return err
		}
		if rel, err := filepath.Rel(i.Root, dst); err != nil || !filepath.IsLocal(rel) {
			return errors.New(locales.ProcessorOutputEscapesInstanceDirectory())
		}
		outputs[dst] = hash
	}
	if len(outputs) > 0 {
		complete := true
		for name, sum := range outputs {
			complete = complete && fsutil.Complete(name, sum, 0)
		}
		if complete {
			return nil
		}
	}
	var cp []string
	for _, coordinate := range append([]string{p.Jar}, p.Classpath...) {
		name, err := minecraft.MavenPath(coordinate)
		if err != nil {
			return err
		}
		dst, err := fsutil.SafePath(filepath.Join(i.Root, "libraries"), name)
		if err != nil {
			return err
		}
		if !fsutil.Complete(dst, "", 0) {
			return fmt.Errorf(locales.MissingProcessorDependency(), coordinate)
		}
		cp = append(cp, dst)
	}
	main, err := mainClass(ctx, cp[0])
	if err != nil {
		return err
	}
	args := []string{"-Xms32m", "-Xmx512m", "-Djava.awt.headless=true", "-cp", strings.Join(cp, string(os.PathListSeparator)), main}
	for _, arg := range p.Args {
		v, err := i.value(arg, vars, work, 0)
		if err != nil {
			return err
		}
		args = append(args, v)
	}
	cmd := exec.CommandContext(ctx, i.Java, args...)
	cmd.Dir = i.Root
	output := &tailOutput{onLine: i.OnLine}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s：%w\n%s", p.Jar, err, output.text)
	}
	for name, sum := range outputs {
		if !fsutil.Complete(name, sum, 0) {
			return fmt.Errorf(locales.ProcessorOutputChecksumMismatch(), filepath.Base(name))
		}
	}
	return nil
}

func mainClass(ctx context.Context, filename string) (string, error) {
	z, err := fsutil.OpenZip(ctx, filename)
	if err != nil {
		return "", err
	}
	defer func() { _ = z.Close() }()
	for _, f := range z.File {
		if strings.EqualFold(f.Name, "META-INF/MANIFEST.MF") {
			data, err := fsutil.ReadZipFile(ctx, f, 1<<20)
			if err != nil {
				return "", err
			}
			scanner := bufio.NewScanner(bytes.NewReader(data))
			var main string
			continuation := false
			for scanner.Scan() {
				line := strings.TrimSuffix(scanner.Text(), "\r")
				if strings.HasPrefix(line, " ") && continuation {
					main += strings.TrimPrefix(line, " ")
					continue
				}
				continuation = false
				if v, ok := strings.CutPrefix(line, "Main-Class: "); ok {
					main = v
					continuation = true
				}
			}
			if err := scanner.Err(); err != nil {
				return "", err
			}
			if main != "" {
				return main, nil
			}
		}
	}
	return "", fmt.Errorf(locales.ProcessorHasNoMainClass(), filename)
}

type tailOutput struct {
	text   string
	onLine func(string)
}

func (t *tailOutput) Write(p []byte) (int, error) {
	t.text += string(p)
	if len(t.text) > 8192 {
		t.text = t.text[len(t.text)-8192:]
	}
	if t.onLine != nil {
		t.onLine(strings.TrimSpace(string(p)))
	}
	return len(p), nil
}
