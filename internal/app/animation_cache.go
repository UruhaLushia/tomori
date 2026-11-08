package app

const maxAnimationCacheBytes = 8 << 20

func storeAnimation(cache map[string][]byte, key string, raw []byte) {
	if len(raw) == 0 || len(raw) > maxAnimationCacheBytes {
		return
	}
	used := 0
	for id, value := range cache {
		if id != key {
			used += len(value)
		}
	}
	if used+len(raw) <= maxAnimationCacheBytes {
		cache[key] = raw
	}
}
