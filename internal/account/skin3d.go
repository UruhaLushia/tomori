package account

import (
	"image"
	"image/color"
	"math"
	"sort"
)

type skinPoint struct{ x, y, z float64 }
type skinUV struct{ u, v float64 }
type skinRect struct{ u, v, w, h float64 }

type skinFace struct {
	vertices [4]skinPoint
	uv       [4]skinUV
	normal   skinPoint
	texture  image.Image
	depth    float64
	light    float64
}

// RenderSkin renders the Minecraft player model with its actual skin texture.
// It is intentionally self-contained so the preview works without a browser
// or a separate graphics runtime.
func RenderSkin(raw, capeRaw []byte, variant string, width, height int, yaw, pitch float32) (image.Image, error) {
	skin, err := decodeSkin(raw)
	if err != nil {
		return nil, err
	}
	if variant != "slim" {
		variant = "classic"
	}
	var cape image.Image
	if len(capeRaw) > 0 {
		cape, _ = decodeSkin(capeRaw)
	}
	width, height = max(1, width), max(1, height)
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	depth := make([]float64, width*height)
	for i := range depth {
		depth[i] = -math.MaxFloat64
	}

	scale := float64(skin.Bounds().Dx()) / 64
	faces := make([]skinFace, 0, 72)
	add := func(min, max skinPoint, rects [6]skinRect, outer bool) {
		addSkinCuboid(&faces, min, max, rects, scale, outer)
	}
	add(skinPoint{-4, 20, -4}, skinPoint{4, 28, 4}, skinCuboidRects(8, 8, 8, 8), false)
	headOuter := skinCuboidRects(40, 8, 8, 8)
	add(skinPoint{-4.25, 19.75, -4.25}, skinPoint{4.25, 28.25, 4.25}, headOuter, true)
	add(skinPoint{-4, 8, -2}, skinPoint{4, 20, 2}, bodyRects(), false)
	add(skinPoint{-4.25, 7.75, -2.25}, skinPoint{4.25, 20.25, 2.25}, bodyOuterRects(), true)

	arm := 4.0
	if variant == "slim" {
		arm = 3
	}
	rightArm := armRects(arm, false)
	leftArm := armRects(arm, true)
	add(skinPoint{-4 - arm, 8, -2}, skinPoint{-4, 20, 2}, rightArm, false)
	add(skinPoint{4, 8, -2}, skinPoint{4 + arm, 20, 2}, leftArm, false)
	add(skinPoint{-4.25 - arm, 7.75, -2.25}, skinPoint{-3.75, 20.25, 2.25}, armOuterRects(arm, false), true)
	add(skinPoint{3.75, 7.75, -2.25}, skinPoint{4.25 + arm, 20.25, 2.25}, armOuterRects(arm, true), true)
	leg := legRects(false, false)
	leftLeg := legRects(true, false)
	add(skinPoint{-4, -4, -2}, skinPoint{0, 8, 2}, leg, false)
	add(skinPoint{0, -4, -2}, skinPoint{4, 8, 2}, leftLeg, false)
	add(skinPoint{-4.25, -4.25, -2.25}, skinPoint{0.25, 8.25, 2.25}, legRects(false, true), true)
	add(skinPoint{-0.25, -4.25, -2.25}, skinPoint{4.25, 8.25, 2.25}, legRects(true, true), true)
	if cape != nil {
		addCape(&faces, cape)
	}

	yawR, pitchR := float64(yaw)*math.Pi/180, float64(pitch)*math.Pi/180
	visible := faces[:0]
	for i := range faces {
		for j := range faces[i].vertices {
			faces[i].vertices[j] = rotateSkinPoint(faces[i].vertices[j], yawR, pitchR)
		}
		faces[i].normal = rotateSkinPoint(faces[i].normal, yawR, pitchR)
		if faces[i].normal.z <= 0 {
			continue
		}
		for _, p := range faces[i].vertices {
			faces[i].depth += p.z
		}
		faces[i].depth /= 4
		light := faces[i].normal.x*0.35 + faces[i].normal.y*0.75 + faces[i].normal.z*0.65
		faces[i].light = maxFloat(0.62, minFloat(1.12, 0.78+light*0.22))
		visible = append(visible, faces[i])
	}
	faces = visible
	sort.SliceStable(faces, func(i, j int) bool { return faces[i].depth < faces[j].depth })

	modelScale := math.Min(float64(width)/22, float64(height)/36)
	minX, maxX := math.MaxFloat64, -math.MaxFloat64
	minY, maxY := math.MaxFloat64, -math.MaxFloat64
	for _, face := range faces {
		for _, p := range face.vertices {
			minX, maxX = minFloat(minX, p.x), maxFloat(maxX, p.x)
			minY, maxY = minFloat(minY, p.y), maxFloat(maxY, p.y)
		}
	}
	centerX, centerY := (minX+maxX)/2, (minY+maxY)/2
	for _, face := range faces {
		var projected [4]skinPoint
		for i, p := range face.vertices {
			projected[i] = skinPoint{float64(width)/2 + (p.x-centerX)*modelScale, float64(height)/2 - (p.y-centerY)*modelScale, p.z}
		}
		texture := skin
		if face.texture != nil {
			texture = face.texture
		}
		rasterSkinTriangle(out, depth, projected[0], projected[1], projected[2], face.uv[0], face.uv[1], face.uv[2], texture, face.light)
		rasterSkinTriangle(out, depth, projected[0], projected[2], projected[3], face.uv[0], face.uv[2], face.uv[3], texture, face.light)
	}
	return out, nil
}

func addSkinCuboid(faces *[]skinFace, min, max skinPoint, rects [6]skinRect, scale float64, outer bool) {
	_ = outer
	addSkinFace(faces, [4]skinPoint{{min.x, max.y, max.z}, {max.x, max.y, max.z}, {max.x, min.y, max.z}, {min.x, min.y, max.z}}, rects[3], skinPoint{0, 0, 1})
	addSkinFace(faces, [4]skinPoint{{max.x, max.y, min.z}, {min.x, max.y, min.z}, {min.x, min.y, min.z}, {max.x, min.y, min.z}}, rects[5], skinPoint{0, 0, -1})
	addSkinFaceUV(faces, [4]skinPoint{{min.x, max.y, max.z}, {min.x, max.y, min.z}, {min.x, min.y, min.z}, {min.x, min.y, max.z}}, rects[2], skinPoint{-1, 0, 0}, sideSkinUVs(rects[2]))
	addSkinFaceUV(faces, [4]skinPoint{{max.x, max.y, min.z}, {max.x, max.y, max.z}, {max.x, min.y, max.z}, {max.x, min.y, min.z}}, rects[4], skinPoint{1, 0, 0}, sideSkinUVs(rects[4]))
	addSkinFace(faces, [4]skinPoint{{min.x, max.y, min.z}, {max.x, max.y, min.z}, {max.x, max.y, max.z}, {min.x, max.y, max.z}}, rects[0], skinPoint{0, 1, 0})
	addSkinFaceUV(faces, [4]skinPoint{{min.x, min.y, max.z}, {max.x, min.y, max.z}, {max.x, min.y, min.z}, {min.x, min.y, min.z}}, rects[1], skinPoint{0, -1, 0}, bottomSkinUVs(rects[1]))
	for i := len(*faces) - 6; i < len(*faces); i++ {
		for j := range (*faces)[i].uv {
			(*faces)[i].uv[j].u *= scale
			(*faces)[i].uv[j].v *= scale
		}
	}
}

func addSkinFace(faces *[]skinFace, vertices [4]skinPoint, rect skinRect, normal skinPoint) {
	addSkinFaceUV(faces, vertices, rect, normal, [4]skinUV{{rect.u, rect.v}, {rect.u + rect.w, rect.v}, {rect.u + rect.w, rect.v + rect.h}, {rect.u, rect.v + rect.h}})
}

func addSkinFaceUV(faces *[]skinFace, vertices [4]skinPoint, rect skinRect, normal skinPoint, uv [4]skinUV) {
	*faces = append(*faces, skinFace{vertices: vertices, uv: uv, normal: normal})
}

func sideSkinUVs(rect skinRect) [4]skinUV {
	return [4]skinUV{{rect.u + rect.w, rect.v}, {rect.u, rect.v}, {rect.u, rect.v + rect.h}, {rect.u + rect.w, rect.v + rect.h}}
}

func bottomSkinUVs(rect skinRect) [4]skinUV {
	return [4]skinUV{{rect.u, rect.v + rect.h}, {rect.u + rect.w, rect.v + rect.h}, {rect.u + rect.w, rect.v}, {rect.u, rect.v}}
}

func skinCuboidRects(x, y, w, h float64) [6]skinRect {
	return [6]skinRect{{x, y - h/2 - 4, w, 4}, {x + w, y - h/2 - 4, w, 4}, {x - 4, y, 4, h}, {x, y, w, h}, {x + w, y, 4, h}, {x + w + 4, y, w, h}}
}

func bodyRects() [6]skinRect {
	return [6]skinRect{{20, 16, 8, 4}, {28, 16, 8, 4}, {16, 20, 4, 12}, {20, 20, 8, 12}, {28, 20, 4, 12}, {32, 20, 8, 12}}
}
func bodyOuterRects() [6]skinRect {
	return [6]skinRect{{20, 32, 8, 4}, {28, 32, 8, 4}, {16, 36, 4, 12}, {20, 36, 8, 12}, {28, 36, 4, 12}, {32, 36, 8, 12}}
}

func legRects(left, outer bool) [6]skinRect {
	x, y := 0.0, 20.0
	if left {
		x, y = 16, 52
	}
	if outer {
		if left {
			x, y = 0, 52
		} else {
			x, y = 0, 36
		}
	}
	return [6]skinRect{{x + 4, y - 4, 4, 4}, {x + 8, y - 4, 4, 4}, {x, y, 4, 12}, {x + 4, y, 4, 12}, {x + 8, y, 4, 12}, {x + 12, y, 4, 12}}
}

func armRects(arm float64, left bool) [6]skinRect {
	x, y := 40.0, 20.0
	if left {
		x, y = 32, 52
	}
	return [6]skinRect{{x + 4, y - 4, arm, 4}, {x + 4 + arm, y - 4, arm, 4}, {x, y, 4, 12}, {x + 4, y, arm, 12}, {x + 4 + arm, y, 4, 12}, {x + 8 + arm, y, arm, 12}}
}

func armOuterRects(arm float64, left bool) [6]skinRect {
	r := armRects(arm, left)
	if left {
		for i := range r {
			r[i].u += 16
		}
	} else {
		for i := range r {
			r[i].v += 16
		}
	}
	return r
}

func addCape(faces *[]skinFace, cape image.Image) {
	_ = cape
	vertices := [4]skinPoint{{-5, 22, -2.7}, {5, 22, -2.7}, {5, 6, -2.7}, {-5, 6, -2.7}}
	addSkinFace(faces, vertices, skinRect{1, 1, 10, 16}, skinPoint{0, 0, -1})
	addSkinFace(faces, [4]skinPoint{{5, 22, -2.65}, {-5, 22, -2.65}, {-5, 6, -2.65}, {5, 6, -2.65}}, skinRect{1, 1, 10, 16}, skinPoint{0, 0, 1})
	(*faces)[len(*faces)-2].texture = cape
	(*faces)[len(*faces)-1].texture = cape
	for i := len(*faces) - 2; i < len(*faces); i++ {
		for j := range (*faces)[i].uv {
			(*faces)[i].uv[j].u *= float64(cape.Bounds().Dx()) / 64
			(*faces)[i].uv[j].v *= float64(cape.Bounds().Dx()) / 64
		}
	}
}

func rotateSkinPoint(p skinPoint, yaw, pitch float64) skinPoint {
	x := p.x*math.Cos(yaw) + p.z*math.Sin(yaw)
	z := -p.x*math.Sin(yaw) + p.z*math.Cos(yaw)
	return skinPoint{x, p.y*math.Cos(pitch) - z*math.Sin(pitch), p.y*math.Sin(pitch) + z*math.Cos(pitch)}
}

func rasterSkinTriangle(out *image.NRGBA, depth []float64, a, b, c skinPoint, ua, ub, uc skinUV, texture image.Image, light float64) {
	minX := maxInt(0, int(math.Floor(minFloat(a.x, minFloat(b.x, c.x)))))
	maxX := minInt(out.Bounds().Dx()-1, int(math.Ceil(maxFloat(a.x, maxFloat(b.x, c.x)))))
	minY := maxInt(0, int(math.Floor(minFloat(a.y, minFloat(b.y, c.y)))))
	maxY := minInt(out.Bounds().Dy()-1, int(math.Ceil(maxFloat(a.y, maxFloat(b.y, c.y)))))
	den := (b.y-c.y)*(a.x-c.x) + (c.x-b.x)*(a.y-c.y)
	if math.Abs(den) < 0.001 {
		return
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			w0 := ((b.y-c.y)*(px-c.x) + (c.x-b.x)*(py-c.y)) / den
			w1 := ((c.y-a.y)*(px-c.x) + (a.x-c.x)*(py-c.y)) / den
			w2 := 1 - w0 - w1
			if w0 < 0 || w1 < 0 || w2 < 0 {
				continue
			}
			z := w0*a.z + w1*b.z + w2*c.z
			index := y*out.Bounds().Dx() + x
			if z <= depth[index] {
				continue
			}
			u := w0*ua.u + w1*ub.u + w2*uc.u
			v := w0*ua.v + w1*ub.v + w2*uc.v
			bounds := texture.Bounds()
			tx := maxInt(bounds.Min.X, minInt(bounds.Max.X-1, int(u)))
			ty := maxInt(bounds.Min.Y, minInt(bounds.Max.Y-1, int(v)))
			col := color.NRGBAModel.Convert(texture.At(tx, ty)).(color.NRGBA)
			if col.A < 8 {
				continue
			}
			col.R, col.G, col.B = uint8(minFloat(255, float64(col.R)*light)), uint8(minFloat(255, float64(col.G)*light)), uint8(minFloat(255, float64(col.B)*light))
			blendSkinPixel(out, x, y, col)
			depth[index] = z
		}
	}
}

func blendSkinPixel(out *image.NRGBA, x, y int, src color.NRGBA) {
	dst := out.NRGBAAt(x, y)
	sa, da := uint32(src.A), uint32(dst.A)
	outA := sa + da*(255-sa)/255
	if outA == 0 {
		return
	}
	out.SetNRGBA(x, y, color.NRGBA{R: uint8((uint32(src.R)*sa + uint32(dst.R)*da*(255-sa)/255) / outA), G: uint8((uint32(src.G)*sa + uint32(dst.G)*da*(255-sa)/255) / outA), B: uint8((uint32(src.B)*sa + uint32(dst.B)*da*(255-sa)/255) / outA), A: uint8(outA)})
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
