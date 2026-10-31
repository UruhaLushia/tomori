package page

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
)

const instanceHoldDelay = 400 * time.Millisecond

type instanceHold struct {
	started time.Time
	x, y    float32
	held    bool
	armed   bool
	moved   bool
}

type instanceReorder struct {
	preview      instanceInsertion
	slots        []instanceSlot
	generation   uint64
	hold         instanceHold
	key          string
	pointer      ui.Point
	offset       ui.Point
	card         ui.Rect
	dragging     bool
	focusPending bool
	scroll       ui.ScrollState
	scrollAt     time.Time
}

type instanceSlot struct {
	index  int
	handle ui.Handle
}

type instanceSlotHit struct {
	key   string
	index int
	after bool
}

type instanceInsertion struct {
	key      string
	from, to int
}

// Keep layout and hit targets fixed throughout the drag. Only the cards'
// paint positions move; pointer capture stays on the grid.
func (s instanceInsertion) destination(i int) int {
	if s.key == "" || i == s.from {
		return i
	}
	if s.from < s.to && i > s.from && i <= s.to {
		return i - 1
	}
	if s.to < s.from && i >= s.to && i < s.from {
		return i + 1
	}
	return i
}

type instanceGridGeometry struct {
	cols   int
	dx, dy float32
}

func (r *instanceReorder) geometry(c *ui.Context, rowHeight float32) instanceGridGeometry {
	g := instanceGridGeometry{cols: 1, dy: rowHeight + kit.S8}
	var first ui.Rect
	firstIndex := 0
	for _, slot := range r.slots {
		bounds := slot.handle.Bounds(c)
		if bounds.W <= 0 || bounds.H <= 0 {
			continue
		}
		if first.W == 0 {
			first, firstIndex = bounds, slot.index
			continue
		}
		if bounds.Y > first.Y+1 {
			g.cols, g.dy = slot.index-firstIndex, bounds.Y-first.Y
			break
		}
		g.cols++
		if g.dx == 0 {
			g.dx = bounds.X - first.X
		}
	}
	return g
}

func (g instanceGridGeometry) offset(from, to int) ui.Point {
	return ui.Point{
		X: float32(to%g.cols-from%g.cols) * g.dx,
		Y: float32(to/g.cols-from/g.cols) * g.dy,
	}
}

func insertionAt(installed []minecraft.Instance, previous instanceInsertion, hit instanceSlotHit) instanceInsertion {
	from := slices.IndexFunc(installed, func(v minecraft.Instance) bool { return v.Key() == hit.key })
	if from < 0 {
		return instanceInsertion{}
	}
	gap := from
	if previous.key == hit.key && previous.from == from {
		gap = previous.to
	}
	to := hit.index
	if to != gap {
		if hit.after {
			to++
		}
		if to > gap {
			to--
		}
	}
	return instanceInsertion{hit.key, from, max(0, min(to, len(installed)-1))}
}

// The grid owns pointer capture, so virtualization cannot drop the gesture.
// A blur synthesizes PointerUp; Focused distinguishes it from a real release.
func (p *Instances) instanceReorderInput(c *ui.Context, grid ui.Element, installed []minecraft.Instance, actions InstancesActions) {
	move := actions.Reorder
	r := &p.reorder
	if move == nil {
		r.cancel()
		return
	}
	grid.Focusable().TrackScroll(&r.scroll)
	if r.focusPending {
		grid.Focus()
		r.focusPending = false
	}
	grid.HandleInput(func(ev ui.InputEvent) bool {
		bounds := p.grid.Bounds(c)
		point := ui.Point{X: ev.X, Y: ev.Y}
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				r.cancel()
				return false
			}
			r.pointer = point
			hit, card := r.hit(c, bounds)
			if hit.index < 0 || hit.index >= len(installed) {
				return false
			}
			r.cancel()
			r.pointer = point
			r.key = installed[hit.index].Key()
			r.hold = instanceHold{started: ev.Time, x: ev.X, y: ev.Y, held: true}
			r.card = card
			r.offset = ui.Point{X: bounds.X + ev.X - card.X, Y: bounds.Y + ev.Y - card.Y}
			r.focusPending = true
			return true
		case ui.InputPointerMove:
			if !r.hold.held {
				return false
			}
			r.pointer = point
			dx, dy := ev.X-r.hold.x, ev.Y-r.hold.y
			if !r.hold.armed && dx*dx+dy*dy > 36 {
				r.hold.moved = true
			}
			if r.hold.armed && dx*dx+dy*dy >= 16 {
				r.dragging = true
			}
			return true
		case ui.InputPointerUp:
			if !r.hold.held {
				return false
			}
			r.pointer = point
			if p.grid.Focused(c) {
				hit, _ := r.hit(c, bounds)
				if r.dragging && insideGrid(bounds, r.pointer) {
					insertion := r.preview
					if hit.index >= 0 {
						hit.key = r.key
						insertion = insertionAt(installed, insertion, hit)
					}
					if insertion.key != "" && insertion.from != insertion.to {
						before, index := "", insertion.to
						if index >= insertion.from {
							index++
						}
						if index < len(installed) {
							before = installed[index].Key()
						}
						move(insertion.key, before)
						r.generation++
					}
				} else if !r.hold.armed && !r.hold.moved && hit.index >= 0 && hit.index < len(installed) && installed[hit.index].Key() == r.key {
					p.Open(r.key)
				}
			}
			r.cancel()
			return true
		case ui.InputKeyDown:
			if r.hold.held && (ev.Key == ui.KeyEscape || ev.Key == ui.KeyTab) {
				r.cancel()
				return ev.Key == ui.KeyEscape
			}
		}
		return false
	})
}

func (r *instanceReorder) cancel() {
	r.hold, r.preview = instanceHold{}, instanceInsertion{}
	r.key, r.dragging, r.focusPending, r.scrollAt = "", false, false, time.Time{}
	r.pointer, r.offset, r.card = ui.Point{}, ui.Point{}, ui.Rect{}
}

func insideGrid(bounds ui.Rect, point ui.Point) bool {
	return point.X >= 0 && point.X < bounds.W && point.Y >= 0 && point.Y < bounds.H
}

func (r *instanceReorder) hit(c *ui.Context, grid ui.Rect) (instanceSlotHit, ui.Rect) {
	x, y := grid.X+r.pointer.X, grid.Y+r.pointer.Y
	for _, slot := range r.slots {
		b := slot.handle.Bounds(c)
		if b.W > 0 && b.H > 0 && x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H {
			after := x >= b.X+b.W/2
			if b.W > grid.W*.9 {
				after = y >= b.Y+b.H/2
			}
			return instanceSlotHit{index: slot.index, after: after}, b
		}
	}
	return instanceSlotHit{index: -1}, ui.Rect{}
}

func (p *Instances) prepareInstanceReorder(c *ui.Context, installed []minecraft.Instance, enabled bool) {
	r := &p.reorder
	if !enabled || r.hold.held && !r.focusPending && !p.grid.Focused(c) {
		r.cancel()
	}
	if !r.hold.held {
		return
	}
	if !r.hold.armed && !r.hold.moved {
		remaining := instanceHoldDelay - c.Now().Sub(r.hold.started)
		if remaining <= 0 {
			r.hold.armed = true
		} else {
			c.After(remaining)
		}
	}
	if !r.dragging {
		return
	}
	bounds := p.grid.Bounds(c)
	if !insideGrid(bounds, r.pointer) {
		r.preview = instanceInsertion{}
		return
	}
	hit, _ := r.hit(c, bounds)
	if hit.index >= 0 {
		hit.key = r.key
		r.preview = insertionAt(installed, r.preview, hit)
	}
	// Match native drag scrolling speed, using the grid's public scroll state.
	var speed float32
	if r.pointer.Y < 32 {
		speed = -(32 - r.pointer.Y) * 30
	}
	if bottom := bounds.H - r.pointer.Y; bottom < 32 {
		speed = (32 - bottom) * 30
	}
	if !r.scrollAt.IsZero() && speed != 0 {
		dt := min(c.Now().Sub(r.scrollAt).Seconds(), .05)
		r.scroll.Y = max(0, min(r.scroll.MaxY, r.scroll.Y+speed*float32(dt)))
	}
	r.scrollAt = c.Now()
	if speed < 0 && r.scroll.Y > 0 || speed > 0 && r.scroll.Y < r.scroll.MaxY {
		c.After(time.Second / 60)
	}
}

func (p *Instances) instanceDragPreview(c *ui.Context, installed []minecraft.Instance, data InstancesData) {
	r := &p.reorder
	if !r.dragging || !r.hold.held {
		return
	}
	index := slices.IndexFunc(installed, func(v minecraft.Instance) bool { return v.Key() == r.key })
	if index < 0 {
		r.cancel()
		return
	}
	bounds := p.grid.Bounds(c)
	ui.Overlay(c, func() {
		c.Key("instance-drag-preview")
		ui.Box(c).Absolute().Left(bounds.X+r.pointer.X-r.offset.X).Top(bounds.Y+r.pointer.Y-r.offset.Y).
			Size(r.card.W, r.card.H).Opacity(.92).PassThrough().Children(func() {
			if p.inline {
				kit.RowButton(c, "", false, nil).Fill().Padding(0, 8).Gap(12).Radius(kit.RadiusM).Background(kit.Surface).Children(func() { p.libraryListBody(c, installed[index], data) })
			} else {
				kit.InstanceCell(c, libraryCellData(installed[index], data), nil)
			}
		})
	})
}
