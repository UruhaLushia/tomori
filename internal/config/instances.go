package config

// InstanceSettings are enabled explicitly; disabled entries retain their values.
// Keys identify the installation path, not just its Minecraft version.
type InstanceSettings struct {
	Enabled bool `json:"enabled"`
	Game    Game `json:"game"`
	Java    Java `json:"java"`
}

func (c Config) GameForInstance(key string) Game {
	if settings, ok := c.Instances[key]; ok && settings.Enabled {
		return settings.Game
	}
	return c.Game
}

func (g *Game) normalize() {
	g.Execution.TimeoutSeconds = g.Execution.Timeout()
	g.MemoryMiB = max(512, g.MemoryMiB)
	if g.Width < 320 {
		g.Width = 854
	}
	if g.Height < 240 {
		g.Height = 480
	}
	g.Width = min(g.Width, 16384)
	g.Height = min(g.Height, 16384)
	if !g.ProcessPriority.Valid() {
		g.ProcessPriority = PriorityNormal
	}
}
