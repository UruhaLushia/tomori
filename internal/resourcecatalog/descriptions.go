package resourcecatalog

import (
	"context"
	"net/url"
	"strings"
	"sync"

	"github.com/egoist/mygo/i18n"
)

const descriptionTranslationAPI = "https://mod.mcimirror.top/translate"

var descriptionTranslations = struct {
	sync.RWMutex
	values map[string]string
}{values: make(map[string]string)}

func (c Client) TranslateDescriptions(ctx context.Context, projects []Project) []Project {
	if c.Fetcher == nil || !strings.HasPrefix(strings.ToLower(i18n.Locale()), "zh") {
		return projects
	}
	result := append([]Project(nil), projects...)
	jobs := make(chan int)
	var group sync.WaitGroup
	for range min(4, len(result)) {
		group.Go(func() {
			for index := range jobs {
				project := result[index]
				if project.ID == "" || project.Description == "" || hasChinese(project.Description) {
					continue
				}
				translated, ok := c.translateDescription(ctx, project.Source, project.ID)
				if ok {
					result[index].TranslatedDescription = translated
				}
			}
		})
	}
	for index := range result {
		jobs <- index
	}
	close(jobs)
	group.Wait()
	return result
}

func (c Client) translateDescription(ctx context.Context, source Source, id string) (string, bool) {
	if !source.Valid() || id == "" {
		return "", false
	}
	key := string(source) + ":" + id
	descriptionTranslations.RLock()
	translated, ok := descriptionTranslations.values[key]
	descriptionTranslations.RUnlock()
	if ok {
		return translated, true
	}
	provider := ""
	switch source {
	case CurseForge:
		provider = "curseforge"
	case Modrinth:
		provider = "modrinth"
	}
	var response struct {
		Translated string `json:"translated"`
	}
	endpoint := descriptionTranslationAPI + "/" + provider + "/" + url.PathEscape(id)
	if err := c.Fetcher.GetJSONLimited(ctx, endpoint, &response, 1<<20); err != nil {
		return "", false
	}
	translated = strings.TrimSpace(response.Translated)
	if translated == "" {
		return "", false
	}
	descriptionTranslations.Lock()
	descriptionTranslations.values[key] = translated
	descriptionTranslations.Unlock()
	return translated, true
}
