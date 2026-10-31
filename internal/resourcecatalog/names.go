package resourcecatalog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

const namesURL = "https://raw.githubusercontent.com/HMCL-dev/HMCL/main/HMCL/src/main/resources/assets/addon/mod.csv"
const namesLimit = 8 << 20
const namesMaxRows = 50000
const namesFresh = 7 * 24 * time.Hour

var nameFailures struct {
	sync.Mutex
	paths map[string]time.Time
}

func namesRetry(cache string) bool {
	nameFailures.Lock()
	defer nameFailures.Unlock()
	return time.Since(nameFailures.paths[cache]) >= time.Minute
}

func namesFailed(cache string, failed bool) {
	nameFailures.Lock()
	defer nameFailures.Unlock()
	if !failed {
		delete(nameFailures.paths, cache)
		return
	}
	if nameFailures.paths == nil {
		nameFailures.paths = make(map[string]time.Time)
	}
	if len(nameFailures.paths) >= 8 {
		var oldest string
		for path, stamp := range nameFailures.paths {
			if oldest == "" || stamp.Before(nameFailures.paths[oldest]) {
				oldest = path
			}
		}
		delete(nameFailures.paths, oldest)
	}
	nameFailures.paths[cache] = time.Now()
}

type NameSuggestion struct{ Name, Query string }
type nameRecord struct{ CurseSlug, ModrinthSlug, Name, English, Abbreviation, ModIDs string }
type nameMatch struct {
	NameSuggestion
	score int
}

func scanNames(ctx context.Context, input io.Reader, visit func(nameRecord)) error {
	limited := &io.LimitedReader{R: input, N: namesLimit + 1}
	reader := bufio.NewScanner(limited)
	reader.Buffer(make([]byte, 4096), 64<<10)
	count := 0
	for reader.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := reader.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var row [8]string
		fields := 0
		// Upstream keeps literal quotes and separates fields with unescaped commas.
		for field := range strings.SplitSeq(line, ",") {
			if fields == len(row) {
				return errors.New(locales.NameCatalogIsInvalidOrTooLarge())
			}
			row[fields] = field
			fields++
		}
		count++
		if count > namesMaxRows || fields < 7 {
			return errors.New(locales.NameCatalogIsInvalidOrTooLarge())
		}
		for _, field := range row {
			if len(field) > 8192 || !utf8.ValidString(field) || strings.ContainsRune(field, 0) {
				return errors.New(locales.NameCatalogIsInvalidOrTooLarge())
			}
		}
		if len(row[1]) > 256 || len(row[2]) > 256 || len(row[3]) > 512 || len(row[4]) > 512 || len(row[5]) > 128 {
			return errors.New(locales.NameCatalogIsInvalidOrTooLarge())
		}
		if visit != nil {
			visit(nameRecord{row[1], row[2], row[3], row[4], row[5], row[6]})
		}
	}
	if err := reader.Err(); err != nil {
		return err
	}
	if limited.N <= 0 || count == 0 {
		return errors.New(locales.NameCatalogIsInvalidOrTooLarge())
	}
	return nil
}

func openCachedNames(cacheDir string) (*os.File, bool, error) {
	root, err := os.OpenRoot(cacheDir)
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	info, err := root.Lstat(download.NamesCache)
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > namesLimit {
		return nil, false, errors.New(locales.NameCatalogIsInvalidOrTooLarge())
	}
	f, err := root.Open(download.NamesCache)
	return f, time.Since(info.ModTime()) < namesFresh, err
}

func (c Client) names(ctx context.Context, refresh bool) (*os.File, error) {
	return c.cachedNames(ctx, refresh, nil)
}

func (c Client) cachedNames(ctx context.Context, refresh bool, invalid *os.File) (*os.File, error) {
	if c.Fetcher == nil || c.Fetcher.CacheDir == "" {
		return nil, os.ErrNotExist
	}
	cache := c.Fetcher.CacheDir
	f, fresh, err := openCachedNames(cache)
	valid := func(current *os.File) bool {
		if invalid == nil {
			return true
		}
		old, oldErr := invalid.Stat()
		now, nowErr := current.Stat()
		return oldErr == nil && nowErr == nil && !os.SameFile(old, now)
	}
	if !refresh || err == nil && fresh && valid(f) {
		return f, err
	}
	if !namesRetry(cache) {
		if f != nil {
			return f, nil
		}
		return nil, os.ErrNotExist
	}
	updateCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	release, err := c.Fetcher.Lock(updateCtx, "resource-names", filepath.Join(cache, download.NamesCache))
	if err != nil {
		if f != nil {
			return f, nil
		}
		return nil, err
	}
	defer release()
	if current, fresh, readErr := openCachedNames(cache); readErr == nil {
		if fresh && valid(current) {
			if f != nil {
				_ = f.Close()
			}
			return current, nil
		}
		_ = current.Close()
	}
	if !namesRetry(cache) {
		if f != nil {
			return f, nil
		}
		return nil, os.ErrNotExist
	}
	raw, updateErr := c.Fetcher.GetLimited(updateCtx, namesURL, namesLimit)
	if updateErr == nil {
		updateErr = scanNames(updateCtx, bytes.NewReader(raw), nil)
	}
	if updateErr == nil {
		updateErr = fsutil.WriteAtomic(filepath.Join(cache, download.NamesCache), raw)
	}
	if updateErr != nil {
		if ctx.Err() == nil {
			namesFailed(cache, true)
		}
		if f != nil {
			return f, nil
		}
		return nil, updateErr
	}
	namesFailed(cache, false)
	if f != nil {
		_ = f.Close()
	}
	f, _, err = openCachedNames(cache)
	return f, err
}

func normalizeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func hasChinese(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func (r nameRecord) query(source Source) string {
	if r.English != "" {
		query, _, _ := strings.Cut(r.English, " / ")
		return strings.TrimSpace(query)
	}
	slug := r.ModrinthSlug
	if source == CurseForge {
		slug = r.CurseSlug
	}
	return strings.ReplaceAll(slug, "-", " ")
}

func nameScore(record nameRecord, query string, chinese bool) int {
	best := 4
	check := func(value string, partial bool) {
		value = normalizeName(value)
		switch {
		case value == query:
			best = 0
		case partial && strings.HasPrefix(value, query):
			best = min(best, 1)
		case partial && strings.Contains(value, query):
			best = min(best, 2)
		}
	}
	if chinese {
		check(record.Name, true)
		if best != 0 && strings.ContainsAny(record.Name, "/;|") {
			for part := range strings.FieldsFuncSeq(record.Name, func(r rune) bool { return r == '/' || r == ';' || r == '|' }) {
				check(part, true)
			}
		}
	} else {
		check(record.Abbreviation, false)
		for id := range strings.SplitSeq(record.ModIDs, ";") {
			if id != "" {
				check(id, false)
			}
		}
	}
	return best
}

func lookupNames(ctx context.Context, f *os.File, source Source, query string) ([]nameMatch, error) {
	normalized, chinese := normalizeName(query), hasChinese(query)
	if normalized == "" || len(query) > 256 || !chinese && len(normalized) > 16 {
		return nil, nil
	}
	var matches []nameMatch
	err := scanNames(ctx, f, func(record nameRecord) {
		score := nameScore(record, normalized, chinese)
		if score > 2 {
			return
		}
		q := record.query(source)
		if q == "" || normalizeName(q) == normalized {
			return
		}
		found := false
		for i, previous := range matches {
			if strings.EqualFold(previous.Query, q) {
				if score >= previous.score {
					return
				}
				matches[i] = nameMatch{NameSuggestion{record.Name, q}, score}
				found = true
				break
			}
		}
		if !found {
			matches = append(matches, nameMatch{NameSuggestion{record.Name, q}, score})
		}
		slices.SortStableFunc(matches, func(a, b nameMatch) int {
			if a.score != b.score {
				return a.score - b.score
			}
			return len(a.Name) - len(b.Name)
		})
		if len(matches) > 6 {
			matches = matches[:6]
		}
	})
	return matches, err
}

func decorateNames(ctx context.Context, f *os.File, source Source, projects []Project) error {
	if len(projects) == 0 {
		return nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	bySlug := make(map[string][]int, len(projects))
	for i, project := range projects {
		if project.Slug != "" {
			slug := strings.ToLower(project.Slug)
			bySlug[slug] = append(bySlug[slug], i)
		}
	}
	ambiguous := make([]bool, len(projects))
	err := scanNames(ctx, f, func(record nameRecord) {
		slug := record.ModrinthSlug
		if source == CurseForge {
			slug = record.CurseSlug
		}
		if hasChinese(record.Name) {
			for _, index := range bySlug[strings.ToLower(slug)] {
				project := &projects[index]
				if project.TranslatedName != "" && project.TranslatedName != record.Name {
					ambiguous[index] = true
				}
				if !ambiguous[index] {
					project.TranslatedName = record.Name
				}
			}
		}
	})
	for i := range projects {
		if err != nil || ambiguous[i] {
			projects[i].TranslatedName = ""
		}
	}
	return err
}

func LocalNames(ctx context.Context, cacheDir string, entries []resource.Entry) error {
	if len(entries) == 0 {
		return nil
	}
	for i := range entries {
		entries[i].TranslatedName, entries[i].SearchAliases = "", ""
	}
	f, _, err := openCachedNames(cacheDir)
	if err != nil {
		return nil
	}
	defer f.Close()
	byID := make(map[string][]int, len(entries))
	ambiguous := make([]bool, len(entries))
	for i, entry := range entries {
		if entry.ID != "" {
			byID[strings.ToLower(entry.ID)] = append(byID[strings.ToLower(entry.ID)], i)
		}
	}
	err = scanNames(ctx, f, func(record nameRecord) {
		for id := range strings.SplitSeq(record.ModIDs, ";") {
			if id == "" {
				continue
			}
			for _, index := range byID[strings.ToLower(id)] {
				entry := &entries[index]
				if hasChinese(record.Name) && entry.TranslatedName != "" && entry.TranslatedName != record.Name {
					ambiguous[index] = true
				}
				if ambiguous[index] {
					entry.TranslatedName = ""
				} else if hasChinese(record.Name) {
					entry.TranslatedName = record.Name
				}
				entry.SearchAliases = record.Abbreviation + " " + record.English
			}
		}
	})
	if err != nil {
		for i := range entries {
			entries[i].TranslatedName, entries[i].SearchAliases = "", ""
		}
	}
	return err
}
