package kit

import "github.com/egoist/mygo/ui"

func svg(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	Coffee = svg(`<path d="M4 9h13v6a5 5 0 0 1-5 5H9a5 5 0 0 1-5-5zM17 9h1a3 3 0 0 1 0 6h-1M7 3v3M11 2v4M15 3v3"/>`)
	Cube   = svg(`<path d="M12 2.9 20 7.3v9.4L12 21.1 4 16.7V7.3z"/><path d="M4 7.3 12 11.7l8-4.4"/><path d="M12 11.7v9.4"/>`)

	Sliders = svg(`<path d="M4 7h9"/><path d="M17 7h3"/><circle cx="15" cy="7" r="2"/><path d="M4 17h4"/><path d="M12 17h8"/><circle cx="10" cy="17" r="2"/>`)

	Play     = svg(`<path d="M8 5.2 19 12 8 18.8z" fill="currentColor" stroke="none"/>`)
	Download = svg(`<path d="M12 3v11m-4-4 4 4 4-4"/><path d="M4 15v5h16v-5"/>`)

	User   = svg(`<circle cx="12" cy="8" r="3.2"/><path d="M5 20v-2a7 7 0 0 1 14 0v2"/>`)
	Clock  = svg(`<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`)
	Search = svg(`<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>`)

	ChevronRight = svg(`<path d="m9 5 7 7-7 7"/>`)
	ChevronDown  = svg(`<path d="m6 9 6 6 6-6"/>`)
	UpDown       = svg(`<path d="m8 9 4-4 4 4M8 15l4 4 4-4"/>`)
	ArrowLeft    = svg(`<path d="M20 12H4m6-6-6 6 6 6"/>`)
	ArrowRight   = svg(`<path d="M4 12h16m-6-6 6 6-6 6"/>`)
	ArrowUp      = svg(`<path d="M12 20V4m-6 6 6-6 6 6"/>`)
	Folder       = svg(`<path d="M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7Z"/>`)
	Image        = svg(`<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8" cy="8" r="1.5"/><path d="m3 16 5-5 4 4 4-6 5 7"/>`)
	Refresh      = svg(`<path d="M20 7v5h-5M4 17v-5h5"/><path d="M6.1 7a7 7 0 0 1 11.5-1.6L20 8M4 16l2.4 2.6A7 7 0 0 0 17.9 17"/>`)
	Home         = svg(`<path d="m3 10 9-7 9 7M5 9v11h5v-7h4v7h5V9"/>`)
	Edit         = svg(`<path d="m15 5 4 4M4 20l4-1L20 7a2.8 2.8 0 0 0-4-4L4 15z"/>`)

	Check = svg(`<path d="M4.5 12.5 9.5 17.5 19.5 6.5"/>`)

	Panel = svg(`<rect x="3.5" y="4.5" width="17" height="15" rx="2.5"/><path d="M9.5 4.5v15"/>`)

	Minus = svg(`<path d="M5 12h14"/>`)
	Plus  = svg(`<path d="M5 12h14M12 5v14"/>`)
	More  = svg(`<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`)

	Grid = svg(`<rect x="4" y="4" width="6.5" height="6.5" rx="1.6"/><rect x="13.5" y="4" width="6.5" height="6.5" rx="1.6"/><rect x="4" y="13.5" width="6.5" height="6.5" rx="1.6"/><rect x="13.5" y="13.5" width="6.5" height="6.5" rx="1.6"/>`)
	List = svg(`<path d="M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01"/>`)

	Sun = svg(`<circle cx="12" cy="12" r="4"/><path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.2 5.2l1.4 1.4M17.4 17.4l1.4 1.4M18.8 5.2l-1.4 1.4M6.6 17.4l-1.4 1.4"/>`)

	External = svg(`<path d="M13.5 4.5h6v6"/><path d="M19.5 4.5 11 13"/><path d="M18 14.5v4a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 4 18.5v-11A1.5 1.5 0 0 1 5.5 6h4"/>`)

	Moon = svg(`<path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z"/>`)

	Square = svg(`<rect x="5.5" y="5.5" width="13" height="13" rx="2"/>`)
	Stop   = svg(`<rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor" stroke="none"/>`)

	Close = svg(`<path d="M6 6l12 12M18 6 6 18"/>`)
	Trash = svg(`<path d="M3 6h18M9 6V4h6v2M5 6l1 15h12l1-15M10 10v7M14 10v7"/>`)

	Globe  = svg(`<circle cx="12" cy="12" r="8.5"/><path d="M3.5 12h17M12 3.5c2.6 2.4 3.9 5.2 3.9 8.5s-1.3 6.1-3.9 8.5M12 3.5C9.4 5.9 8.1 8.7 8.1 12s1.3 6.1 3.9 8.5"/>`)
	Puzzle = svg(`<path d="M5 9h3.6a2.4 2.4 0 1 1 4.8 0H17v3.6a2.4 2.4 0 1 1 0 4.8V20H5z"/>`)
	Layers = svg(`<path d="m12 3.5 8.5 4.75L12 13 3.5 8.25z"/><path d="m3.5 12.5 8.5 4.75 8.5-4.75"/><path d="m3.5 16.25 8.5 4.75 8.5-4.75"/>`)
	Server = svg(`<rect x="3.5" y="4" width="17" height="7" rx="2"/><rect x="3.5" y="13" width="17" height="7" rx="2"/><path d="M7.5 7.5h.01M7.5 16.5h.01"/>`)
)
