package page

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/resource"
)

type resourceSelectionDrag struct {
	active, moved, add   bool
	from, target, last   int
	x, y, startX, startY float32
	base                 []string
}

func (p *Resources) selectResource(entries []resource.Entry, index int, mods ui.Modifiers, checkbox bool) {
	key := entries[index].Relative
	if mods&ui.Shift != 0 {
		from := index
		for i, entry := range entries {
			if entry.Relative == p.selectionAnchor {
				from = i
				break
			}
		}
		if mods&ui.Cmd == 0 {
			p.selected.Clear()
		}
		for i := min(from, index); i <= max(from, index); i++ {
			p.selected.Add(entries[i].Relative)
		}
	} else {
		p.selectionAnchor = key
		if checkbox || mods&ui.Cmd != 0 {
			if p.selected.Has(key) {
				p.selected.Remove(key)
			} else {
				p.selected.Add(key)
			}
		}
	}
	p.focusList = true
}

func (p *Resources) resourceRowInput(c *ui.Context, el ui.Element, index int, entries []resource.Entry, seen *bool) {
	mods := el.ClickModifiers()
	el.OnClick(func() { p.selectResource(entries, index, mods, false) })
	_, _, down := el.Dragged()
	d := &p.selectionDrag
	if down {
		*seen = true
		x, y, _ := el.PointerPosition()
		x, y = el.LocalToWindow(x, y)
		if !d.active {
			*d = resourceSelectionDrag{active: true, from: index, target: index, last: -1, startX: x, startY: y, add: c.Modifiers()&ui.Cmd != 0}
		}
		d.x, d.y = x, y
		if !d.moved && (math.Abs(float64(x-d.startX)) > 4 || math.Abs(float64(y-d.startY)) > 4) {
			d.moved = true
			if d.add {
				for key := range p.selected.All() {
					d.base = append(d.base, key)
				}
			}
		}
	}
	if d.active && d.moved {
		r := el.Bounds()
		if d.y >= r.Y && d.y < r.Y+r.H {
			d.target = index
		}
	}
}

func (p *Resources) finishResourceDrag(c *ui.Context, list ui.Element, entries []resource.Entry, seen bool) {
	d := &p.selectionDrag
	if !seen {
		*d = resourceSelectionDrag{}
		return
	}
	if !d.moved {
		return
	}
	if d.target != d.last {
		p.selected.Clear()
		for _, key := range d.base {
			p.selected.Add(key)
		}
		for i := min(d.from, d.target); i <= max(d.from, d.target); i++ {
			p.selected.Add(entries[i].Relative)
		}
		d.last = d.target
	}
	r := list.Bounds()
	first, last := p.list.Visible()
	if d.y < r.Y+16 && first > 0 {
		p.list.ScrollIntoView(first - 1)
	} else if d.y > r.Y+r.H-16 && last < len(entries)-1 {
		p.list.ScrollIntoView(last + 1)
	}
	c.After(16 * time.Millisecond)
}
