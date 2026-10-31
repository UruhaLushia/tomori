package kit

var (
	interfaceFont string
	logFont       = "monospace"
)

func SetFonts(family, logs string) {
	if logs == "" {
		logs = "monospace"
	}
	if interfaceFont != family || logFont != logs {
		interfaceFont, logFont = family, logs
		cachedTheme = nil
	}
}

func LogFont() string { return logFont }
