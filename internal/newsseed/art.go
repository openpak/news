// SPDX-License-Identifier: AGPL-3.0-only
// Original artwork for the OpenPak channel seed, built on the official
// OpenPak mark. The mark reproduces website/web/static/icon.svg (512-unit
// design: indigo gradient disc, white ring, 270-degree arc, centre dot) —
// the same geometry app/lib/logo.dart renders and the console NRO draws.
// Channel icons swap the centre dot for a glyph; article art composes the
// mark with gaming scenes. Everything is drawn here from deterministic
// math; nothing is copied from games or third-party assets.

package newsseed

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os/exec"
)

type rgb = color.RGBA

func hexRGB(s string) rgb {
	var r, g, b uint8
	fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b)
	return rgb{R: r, G: g, B: b, A: 255}
}

// Brand palette: the logo's indigo gradient plus its tints.
var (
	logoFrom   = hexRGB("#6366F1")
	logoTo     = hexRGB("#4338CA")
	indigoHi   = hexRGB("#818CF8")
	indigoMid  = hexRGB("#4F46E5")
	indigoDeep = hexRGB("#3730A3")
	inkWhite   = hexRGB("#F4F5FF")
	sceneTop   = hexRGB("#2B2470")
	sceneMid   = hexRGB("#141130")
	sceneBot   = hexRGB("#060514")
)

const white = "#FFFFFF"

// ---------- canvas primitives ----------

type canvas struct {
	w, h int
	px   []rgb
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, px: make([]rgb, w*h)}
}

func (c *canvas) At(x, y int) color.Color {
	return c.px[clamp(y, 0, c.h-1)*c.w+clamp(x, 0, c.w-1)]
}
func (c *canvas) ColorModel() color.Model { return color.RGBAModel }
func (c *canvas) Bounds() image.Rectangle { return image.Rect(0, 0, c.w, c.h) }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (c *canvas) set(x, y int, col rgb) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.px[y*c.w+x] = col
}

// over composites src onto dst with alpha a (0..1).
func over(dst, src rgb, a float64) rgb {
	mix := func(d, s uint8) uint8 {
		return uint8(math.Round(float64(s)*a + float64(d)*(1-a)))
	}
	return rgb{R: mix(dst.R, src.R), G: mix(dst.G, src.G), B: mix(dst.B, src.B), A: 255}
}

func (c *canvas) blend(x, y int, col rgb, a float64) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h || a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	c.set(x, y, over(c.px[y*c.w+x], col, a))
}

func (c *canvas) fill(col rgb) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			c.set(x, y, col)
		}
	}
}

// diagGrad paints a diagonal three-stop gradient (top-left to bottom-right).
func (c *canvas) diagGrad(stops ...rgb) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			t := (float64(x)/float64(c.w) + float64(y)/float64(c.h)) / 2
			c.set(x, y, gradAt(stops, t))
		}
	}
}

func gradAt(stops []rgb, t float64) rgb {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	n := len(stops) - 1
	f := t * float64(n)
	i := int(f)
	if i >= n {
		return stops[n]
	}
	local := f - float64(i)
	return lerp(stops[i], stops[i+1], local)
}

func (c *canvas) rect(x0, y0, x1, y1 int, col rgb) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c.set(x, y, col)
		}
	}
}

func (c *canvas) disc(cx, cy int, r float64, col rgb) {
	if r <= 0 {
		return
	}
	for y := cy - int(r) - 1; y <= cy+int(r)+1; y++ {
		for x := cx - int(r) - 1; x <= cx+int(r)+1; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			if math.Sqrt(dx*dx+dy*dy) <= r+0.5 {
				c.set(x, y, col)
			}
		}
	}
}

func (c *canvas) glowDisc(cx, cy int, r float64, col rgb, a float64) {
	// Radial falloff in five shells — cheap deterministic bloom.
	for i := 5; i >= 1; i-- {
		c.discGlow(cx, cy, r*float64(i)/2.2, col, a*0.22)
	}
	c.disc(cx, cy, r, col)
}

func (c *canvas) discGlow(cx, cy int, r float64, col rgb, a float64) {
	for y := cy - int(r) - 1; y <= cy+int(r)+1; y++ {
		for x := cx - int(r) - 1; x <= cx+int(r)+1; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			d := math.Sqrt(dx*dx + dy*dy)
			if d <= r {
				fall := 1 - 0.7*(d/r)
				c.blend(x, y, col, a*fall)
			}
		}
	}
}

func (c *canvas) ring(cx, cy int, r, thick float64, col rgb) {
	if r <= 0 || thick <= 0 {
		return
	}
	ro, ri := r+thick/2+0.5, r-thick/2-0.5
	for y := cy - int(r+thick) - 1; y <= cy+int(r+thick)+1; y++ {
		for x := cx - int(r+thick) - 1; x <= cx+int(r+thick)+1; x++ {
			dx, dy := float64(x-cx), float64(y-cy)
			d := math.Sqrt(dx*dx + dy*dy)
			if d <= ro && d >= ri {
				c.set(x, y, col)
			}
		}
	}
}

// arc strokes a circular arc from a0 to a1 degrees (y-down screen angles:
// 0 = east, 90 = south, 180 = west, 270 = north), with round caps.
func (c *canvas) arc(cx, cy int, r, thick float64, a0, a1 float64, col rgb) {
	if r <= 0 || thick <= 0 {
		return
	}
	steps := int((a1 - a0) / 360 * (2 * math.Pi * r) * 2)
	if steps < 8 {
		steps = 8
	}
	for i := 0; i <= steps; i++ {
		a := (a0 + (a1-a0)*float64(i)/float64(steps)) * math.Pi / 180
		x := cx + int(math.Round(r*math.Cos(a)))
		y := cy + int(math.Round(r*math.Sin(a)))
		c.disc(x, y, thick/2, col)
	}
}

func (c *canvas) line(x0, y0, x1, y1 int, thick float64, col rgb) {
	steps := max(abs(x1-x0), abs(y1-y0), 1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + int(math.Round(float64(x1-x0)*t))
		y := y0 + int(math.Round(float64(y1-y0)*t))
		c.disc(x, y, thick/2, col)
	}
}

// rrect fills a rounded rectangle.
func (c *canvas) rrect(x0, y0, x1, y1 int, rad float64, col rgb) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dx, dy := 0.0, 0.0
			cx, cy := float64(x), float64(y)
			if cx < float64(x0)+rad {
				dx = float64(x0) + rad - cx
			} else if cx > float64(x1)-rad {
				dx = cx - (float64(x1) - rad)
			}
			if cy < float64(y0)+rad {
				dy = float64(y0) + rad - cy
			} else if cy > float64(y1)-rad {
				dy = cy - (float64(y1) - rad)
			}
			if math.Sqrt(dx*dx+dy*dy) <= rad+0.5 {
				c.set(x, y, col)
			}
		}
	}
}

// ---------- the OpenPak mark ----------

// drawMark renders the official mark centred at (cx, cy) at the given size
// (the full 512-unit design scales to `size` pixels). glyph, when non-nil,
// replaces the centre dot and draws in logo units around the centre.
func drawMark(c *canvas, cx, cy int, size int, glyph func(cc *canvas, cx, cy int, u float64)) {
	u := float64(size) / 512.0
	r := float64(size) / 2

	// Background disc with the logo's diagonal gradient.
	for y := -size/2 - 1; y <= size/2+1; y++ {
		for x := -size/2 - 1; x <= size/2+1; x++ {
			dx, dy := float64(x), float64(y)
			if math.Sqrt(dx*dx+dy*dy) > r+0.5 {
				continue
			}
			t := (dx + dy + 2*r) / (4 * r)
			c.set(cx+x, cy+y, over(logoTo, logoFrom, 1-t))
		}
	}

	whiteCol := hexRGB(white)
	ringCol := over(logoTo, whiteCol, 0.9)
	c.ring(cx, cy, 180*u, 20*u, ringCol)
	// The spiral arc: due west, over the north, through east, to due south.
	c.arc(cx, cy, 96*u, 20*u, 180, 450, ringCol)
	c.disc(cx-int(math.Round(96*u)), cy, 10*u, ringCol) // round west cap
	c.disc(cx, cy+int(math.Round(96*u)), 10*u, ringCol) // round south cap

	if glyph != nil {
		glyph(c, cx, cy, u)
	} else {
		c.disc(cx, cy, 28*u, over(logoTo, whiteCol, 0.95))
	}
}

// ---------- centre-dot glyph variants (logo units) ----------

// padGlyph: a small gamepad — bold body, d-pad cross and two buttons.
func padGlyph(c *canvas, cx, cy int, u float64) {
	w := hexRGB(white)
	dark := indigoDeep
	bw, bh := int(148*u), int(94*u)
	c.rrect(cx-bw/2, cy-bh/2, cx+bw/2, cy+bh/2, 42*u, w)
	// d-pad: one bold cross
	px, py := cx-int(34*u), cy+int(2*u)
	c.rect(px-int(12*u), py-int(30*u), px+int(12*u), py+int(30*u), dark)
	c.rect(px-int(30*u), py-int(12*u), px+int(30*u), py+int(12*u), dark)
	// two face buttons
	c.disc(cx+int(28*u), cy-int(16*u), 12*u, dark)
	c.disc(cx+int(46*u), cy+int(14*u), 12*u, dark)
}

// questionGlyph: a bold question mark.
func questionGlyph(c *canvas, cx, cy int, u float64) {
	w := hexRGB(white)
	c.arc(cx-int(4*u), cy-int(18*u), 34*u, 24*u, 170, 440, w)
	c.line(cx-int(4*u), cy+int(16*u), cx+int(2*u), cy+int(30*u), 24*u, w)
	c.disc(cx+int(4*u), cy+int(56*u), 13*u, w)
}

// gearGlyph: a toothed gear ring.
func gearGlyph(c *canvas, cx, cy int, u float64) {
	w := hexRGB(white)
	c.ring(cx, cy, 34*u, 20*u, w)
	for i := 0; i < 6; i++ {
		a := float64(i) * 60 * math.Pi / 180
		c.line(cx+int(math.Round(38*u*math.Cos(a))), cy+int(math.Round(38*u*math.Sin(a))),
			cx+int(math.Round(56*u*math.Cos(a))), cy+int(math.Round(56*u*math.Sin(a))), 22*u, w)
	}
	c.disc(cx, cy, 13*u, w)
}

// ---------- scenes ----------

type scene struct {
	c *canvas
	W int
	H int
	u float64 // design unit: H / 364
}

func newScene(w, h int) *scene {
	return &scene{c: newCanvas(w, h), W: w, H: h, u: float64(h) / 364.0}
}

func (s *scene) backdrop(grid bool) {
	s.c.diagGrad(sceneTop, sceneMid, sceneBot)
	// Deterministic star specks in the upper field.
	for i := 0; i < 42; i++ {
		x := (i*97 + 23) % s.W
		y := (i*53 + 11) % (s.H * 5 / 8)
		a := 0.12 + float64((i*31)%40)/100
		s.c.blend(x, y, indigoHi, a)
		if i%4 == 0 {
			s.c.blend((x+7)%s.W, (y+3)%(s.H*5/8), inkWhite, a*0.7)
		}
	}
	if grid {
		hy := s.H * 68 / 100
		// Glow at the horizon.
		s.c.discGlow(s.W/2, hy, float64(s.W)/3, indigoMid, 0.20)
		// Verticals converging on the vanishing point.
		for i := -14; i <= 14; i++ {
			x0 := s.W/2 + i*s.W/22
			s.c.blendLine(x0, hy, s.W/2+i*s.W/5, s.H, indigoMid, 0.28, 1)
		}
		// Receding horizontals.
		for k, t := 0, 0.06; t < 1; k++ {
			y := hy + int(math.Round(float64(s.H-hy)*t*t))
			s.c.blendLine(0, y, s.W, y, indigoMid, 0.22, 1)
			t += 0.055 * float64(k+1)
		}
		s.c.blendRect(0, hy, s.W, hy+1, indigoHi, 0.5)
	}
}

func (s *scene) bug() {
	size := s.H * 3 / 20
	drawMark(s.c, s.W-size*3/2, s.H-size*3/2, size, nil)
}

func (c *canvas) blendLine(x0, y0, x1, y1 int, col rgb, a float64, thick int) {
	steps := max(abs(x1-x0), abs(y1-y0), 1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + int(math.Round(float64(x1-x0)*t))
		y := y0 + int(math.Round(float64(y1-y0)*t))
		c.blend(x, y, col, a)
		if thick > 1 {
			c.blend(x, y+1, col, a)
		}
	}
}

func (c *canvas) blendRect(x0, y0, x1, y1 int, col rgb, a float64) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c.blend(x, y, col, a)
		}
	}
}

// scene functions paint the central subject; (cx, cy) is the optical centre.

func sceneMark(s *scene, cx, cy int) {
	size := s.H * 3 / 5
	s.c.discGlow(cx, cy, float64(size)/2, indigoMid, 0.5)
	s.c.ring(cx, cy, float64(size)/2+float64(s.H)/22, float64(s.H)/180, over(indigoDeep, hexRGB(white), 0.35))
	drawMark(s.c, cx, cy, size, nil)
}

func sceneBurst(s *scene, cx, cy int) {
	size := s.H * 11 / 20
	for i := 0; i < 24; i++ {
		a := float64(i) * 15 * math.Pi / 180
		r0 := float64(size) * 0.62
		r1 := float64(size) * (0.95 + 0.18*float64(i%3))
		col := indigoMid
		if i%2 == 0 {
			col = indigoHi
		}
		s.c.blendLine(cx+int(r0*math.Cos(a)), cy+int(r0*math.Sin(a)),
			cx+int(r1*math.Cos(a)), cy+int(r1*math.Sin(a)), col, 0.35, 2)
	}
	drawMark(s.c, cx, cy, size, nil)
}

func sceneHorizon(s *scene, cx, cy int) {
	hy := s.H * 68 / 100
	s.c.discGlow(cx, hy, float64(s.W)/3.2, indigoMid, 0.3)
	drawMark(s.c, cx, hy-int(s.H*19/100), s.H*2/5, nil)
	// Faint reflection of the disc below the horizon line.
	for i := 0; i < 5; i++ {
		s.c.blendLine(cx-s.H/5+i*s.H/40, hy+4, cx-s.H/5+i*s.H/40+8, hy+4+i*2, indigoHi, 0.10, 1)
	}
}

func scenePad(s *scene, cx, cy int) {
	u := float64(s.H) / 364 * 1.5
	w := hexRGB(white)
	_ = w
	s.c.glowDisc(cx, cy, float64(s.H)/3, indigoMid, 0.4)
	bw, bh := int(300*u), int(180*u)
	s.c.rrect(cx-bw/2, cy-bh/2, cx+bw/2, cy+bh/2, 70*u, indigoDeep)
	s.c.rrect(cx-bw/2, cy-bh/2, cx+bw/2, cy+bh/2-8*int(u), 70*u, w)
	// d-pad cross
	px, py := cx-int(78*u), cy+int(4*u)
	s.c.rect(px-int(16*u), py-int(52*u), px+int(16*u), py+int(52*u), indigoDeep)
	s.c.rect(px-int(52*u), py-int(16*u), px+int(52*u), py+int(16*u), indigoDeep)
	s.c.rect(px-int(11*u), py-int(45*u), px+int(11*u), py+int(45*u), indigoMid)
	s.c.rect(px-int(45*u), py-int(11*u), px+int(45*u), py+int(11*u), indigoMid)
	// four face buttons
	for i, off := range [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		bx := cx + int(78*u) + off[0]*int(30*u)
		by := cy + int(4*u) + off[1]*int(30*u)
		col := indigoDeep
		if i == 0 {
			col = indigoMid
		}
		s.c.disc(bx, by, 14*u, col)
	}
	// menu pips
	s.c.disc(cx-int(10*u), cy-int(40*u), 7*u, indigoDeep)
	s.c.disc(cx+int(10*u), cy-int(40*u), 7*u, indigoDeep)
}

func sceneLevels(s *scene, cx, cy int) {
	w := hexRGB(white)
	base := s.H * 78 / 100
	widths := s.H / 5
	for i := 0; i < 3; i++ {
		h := s.H * (22 + 14*i) / 100
		x0 := cx - widths*3/2 - s.H/40 + i*(widths+s.H/22)
		col := indigoDeep
		if i == 2 {
			col = indigoHi
		}
		s.c.rrect(x0, base-h, x0+widths, base, float64(s.H)/90, col)
		if i == 2 {
			s.c.disc(x0+widths/2, base-h-s.H/22, float64(s.H)/70, w)
			s.c.discGlow(x0+widths/2, base-h-s.H/22, float64(s.H)/40, indigoHi, 0.5)
		}
	}
}

func scenePodium(s *scene, cx, cy int) {
	w := hexRGB(white)
	base := s.H * 80 / 100
	hts := [3]int{46, 62, 34} // 2nd, 1st, 3rd
	for i, h := range hts {
		x0 := cx - s.H*31/100 + i*s.H*22/100
		col := indigoDeep
		if i == 1 {
			col = indigoMid
		}
		s.c.rrect(x0, base-s.H*h/100, x0+s.H*18/100, base, float64(s.H)/90, col)
		s.c.blendRect(x0, base-s.H*h/100, x0+s.H*18/100, base-s.H*h/100+3, w, 0.6)
	}
	// Sparkle above the tallest step.
	tx, ty := cx-s.H*22/100+9*s.H/100, base-s.H*62/100
	for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		s.c.line(tx, ty, tx+d[0]*s.H/24, ty+d[1]*s.H/24, float64(s.H)/90, w)
	}
	s.c.disc(tx, ty, float64(s.H)/120, w)
	s.c.discGlow(tx, ty, float64(s.H)/30, indigoHi, 0.5)
}

func sceneGear(s *scene, cx, cy int) {
	u := float64(s.H) / 364
	w := hexRGB(white)
	s.c.discGlow(cx, cy, float64(s.H)/2.6, indigoMid, 0.4)
	s.c.ring(cx, cy, 120*u, 44*u, indigoHi)
	for i := 0; i < 10; i++ {
		a := float64(i) * 36 * math.Pi / 180
		s.c.line(cx+int(math.Round(120*u*math.Cos(a))), cy+int(math.Round(120*u*math.Sin(a))),
			cx+int(math.Round(172*u*math.Cos(a))), cy+int(math.Round(172*u*math.Sin(a))), 40*u, indigoHi)
	}
	s.c.disc(cx, cy, 84*u, sceneMid)
	drawMark(s.c, cx, cy, int(120*u), nil)
	_ = w
}

func sceneOrbit(s *scene, cx, cy int) {
	drawMark(s.c, cx, cy, s.H*2/5, nil)
	for k, r := range [3]float64{0.30, 0.42, 0.54} {
		rx, ry := float64(s.H)*r*1.35, float64(s.H)*r
		a := float64(k) * 1.9
		for t := 0.0; t < 360; t += 0.7 {
			th := t * math.Pi / 180
			x := cx + int(math.Round(rx*math.Cos(th)))
			y := cy + int(math.Round(ry*math.Sin(th)))
			s.c.blend(x, y, indigoHi, 0.20)
		}
		px := cx + int(math.Round(rx*math.Cos(a)))
		py := cy + int(math.Round(ry*math.Sin(a)))
		s.c.disc(px, py, float64(s.H)/60, inkWhite)
		s.c.discGlow(px, py, float64(s.H)/32, indigoHi, 0.5)
	}
}

func sceneWrench(s *scene, cx, cy int) {
	w := hexRGB(white)
	th := float64(s.H) / 22
	// Handle: bottom-left to centre, then the open jaw (a 240-degree arc).
	hx1, hy1 := cx-s.H/3, cy+s.H/4
	hx2, hy2 := cx+s.H/22, cy-s.H/22
	s.c.line(hx1, hy1, hx2, hy2, th*1.4, w)
	jr := float64(s.H) / 8
	jx, jy := float64(hx2)+jr/2, float64(hy2)-jr/2
	s.c.arc(int(jx), int(jy), jr, th*1.6, 130, 360, w)
	s.c.disc(int(jx-jr), int(jy), th*0.8, w)
	// A couple of bolts.
	for _, p := range [2][2]int{{cx + s.H/4, cy + s.H/6}, {cx + s.H/3, cy - s.H/12}} {
		s.c.ring(p[0], p[1], float64(s.H)/32, float64(s.H)/70, indigoHi)
	}
}

func scenePower(s *scene, cx, cy int) {
	r := float64(s.H) / 4.4
	th := float64(s.H) / 24
	s.c.discGlow(cx, cy, r*1.9, indigoMid, 0.45)
	w := hexRGB(white)
	s.c.arc(cx, cy, r, th, 210, 330+360, w) // ring with a gap at the top
	capA := 210 * math.Pi / 180
	capB := (330 + 360) * math.Pi / 180
	for _, a := range []float64{capA, capB} {
		s.c.disc(cx+int(math.Round(r*math.Cos(a))), cy+int(math.Round(r*math.Sin(a))), th/2, w)
	}
	s.c.line(cx, cy-int(r)-int(th), cx, cy-int(r*0.25), th, w)
}

func sceneQuestion(s *scene, cx, cy int) {
	u := float64(s.H) / 364 * 2.4
	s.c.discGlow(cx, cy, float64(s.H)/2.4, indigoMid, 0.4)
	questionGlyph(s.c, cx, cy-s.H/40, u)
}

func sceneRefresh(s *scene, cx, cy int) {
	r := float64(s.H) / 4.2
	th := float64(s.H) / 20
	w := hexRGB(white)
	s.c.discGlow(cx, cy, r*1.8, indigoMid, 0.4)
	// Two arcs with clear gaps at the top-left and bottom-right.
	s.c.arc(cx, cy, r, th, 295, 425, w)
	s.c.arc(cx, cy, r, th, 115, 245, w)
	// Tangent arrowheads at each trailing end, pointing along the cycle.
	refreshHead(s, cx, cy, r, th, 425, w)
	refreshHead(s, cx, cy, r, th, 245, w)
}

func refreshHead(s *scene, cx, cy int, r, th float64, aDeg float64, col rgb) {
	pt := func(a, rr float64) (int, int) {
		t := a * math.Pi / 180
		return cx + int(math.Round(rr*math.Cos(t))), cy + int(math.Round(rr*math.Sin(t)))
	}
	tx, ty := pt(aDeg+8, r)
	w1x, w1y := pt(aDeg-18, r+th*0.95)
	w2x, w2y := pt(aDeg-18, r-th*0.95)
	s.c.line(tx, ty, w1x, w1y, th*0.8, col)
	s.c.line(tx, ty, w2x, w2y, th*0.8, col)
}

// ---------- renderers ----------

type motifFunc func(s *scene, cx, cy int)

// sceneMotifs is the registry; per-title painters register in init().
var sceneMotifs = map[string]motifFunc{
	"mark":     sceneMark,
	"burst":    sceneBurst,
	"horizon":  sceneHorizon,
	"pad":      scenePad,
	"levels":   sceneLevels,
	"podium":   scenePodium,
	"gear":     sceneGear,
	"orbit":    sceneOrbit,
	"wrench":   sceneWrench,
	"power":    scenePower,
	"question": sceneQuestion,
	"refresh":  sceneRefresh,
}

func registerMotif(name string, m motifFunc) { sceneMotifs[name] = m }

func sceneByName(name string) (motifFunc, bool, error) {
	m, ok := sceneMotifs[name]
	if !ok {
		return nil, false, fmt.Errorf("unknown motif %q", name)
	}
	return m, name == "horizon", nil
}

// renderArt paints an article card or hero scene at the requested size.
func renderArt(motif string, hue, w, h int) (image.Image, error) {
	_, _, err := sceneByName(motif)
	if err != nil {
		return nil, err
	}
	s := newScene(w, h)
	m, grid, _ := sceneByName(motif)
	s.backdrop(grid)
	m(s, w/2, h*47/100)
	s.bug()
	return s.c, nil
}

// Channel icon motifs: the official mark, or the mark with its centre dot
// replaced by a channel glyph.
func renderIcon(name string, hue int) (image.Image, error) {
	glyphs := map[string]func(*canvas, int, int, float64){
		"mark":          nil,
		"mark-pad":      padGlyph,
		"mark-question": questionGlyph,
		"mark-gear":     gearGlyph,
	}
	glyph, ok := glyphs[name]
	if !ok {
		return nil, fmt.Errorf("unknown icon motif %q (use mark, mark-pad, mark-question or mark-gear)", name)
	}
	c := newCanvas(70, 70)
	drawMark(c, 35, 35, 68, glyph)
	return c, nil
}

// encodeJPEG emits a baseline JFIF 4:4:4 JPEG via libjpeg-turbo's cjpeg,
// matching the encoding the console has accepted in every rendered test.
func encodeJPEG(im image.Image) ([]byte, error) {
	b := im.Bounds()
	var ppm bytes.Buffer
	fmt.Fprintf(&ppm, "P6\n%d %d\n255\n", b.Dx(), b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := im.At(x, y).RGBA()
			ppm.WriteByte(byte(r >> 8))
			ppm.WriteByte(byte(g >> 8))
			ppm.WriteByte(byte(bl >> 8))
		}
	}
	cmd := exec.Command("cjpeg", "-quality", "90", "-baseline", "-sample", "1x1")
	cmd.Stdin = &ppm
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cjpeg (libjpeg-turbo) is required: %w", err)
	}
	return out, nil
}

func lerp(a, b rgb, t float64) rgb {
	mix := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x)*(1-t) + float64(y)*t))
	}
	return rgb{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ---------- per-title and console scenes (all original, generic geometry) ----------

func sceneGlobe(s *scene, cx, cy int) { // shared world
	r := float64(s.H) / 3.6
	s.c.discGlow(cx, cy, r*1.5, indigoMid, 0.4)
	for _, k := range [3]float64{0.35, 0.68, 1.0} {
		s.c.ring(cx, cy, r*k, float64(s.H)/110, indigoHi)
	}
	for _, off := range [3]float64{-0.45, 0, 0.45} {
		s.c.ellipse(cx, cy, r*math.Sqrt(1-off*off), r, float64(s.H)/110, indigoHi)
	}
	s.c.disc(cx+int(r*0.55), cy-int(r*0.55), float64(s.H)/90, inkWhite)
}

// ellipse strokes a parametric axis-aligned ellipse ring.
func (c *canvas) ellipse(cx, cy int, rx, ry, thick float64, col rgb) {
	for i := 0; i <= 220; i++ {
		a := float64(i) / 220 * 2 * math.Pi
		c.disc(cx+int(math.Round(rx*math.Cos(a))), cy+int(math.Round(ry*math.Sin(a))), thick/2, col)
	}
}

func sceneFarm(s *scene, cx, cy int) { // fields
	hy := s.H * 62 / 100
	s.c.discGlow(cx-int(s.H/4), hy-int(s.H/5), float64(s.H)/10, hexRGB("#F5C66B"), 0.5)
	s.c.disc(cx-int(s.H/4), hy-int(s.H/5), float64(s.H)/26, hexRGB("#F5C66B"))
	for i := -6; i <= 6; i++ {
		s.c.blendLine(cx+i*s.H/45, hy, cx+i*s.H/9, s.H, indigoHi, 0.30, 2)
	}
	s.c.blendRect(0, hy, s.W, hy+1, indigoHi, 0.4)
}

func scenePod(s *scene, cx, cy int) { // little spacecraft
	u := float64(s.H) / 364
	w := hexRGB(white)
	s.c.discGlow(cx, cy, float64(s.H)/2.8, indigoMid, 0.4)
	bw, bh := int(120*u), int(200*u)
	s.c.rrect(cx-bw/2, cy-bh/2, cx+bw/2, cy+bh/2, 55*u, w)
	vx, vy, vr := cx, cy-int(30*u), 42*u
	s.c.disc(vx, vy, vr, indigoDeep)
	s.c.disc(vx, vy, vr*0.62, indigoHi)
	s.c.rect(cx-int(14*u), cy+int(58*u), cx+int(14*u), cy+int(92*u), indigoDeep)
}

func sceneFlag(s *scene, cx, cy int) { // racing
	w := hexRGB(white)
	pole := s.H * 55 / 100
	s.c.line(cx-s.H/8, cy-pole/2, cx-s.H/8, cy+pole/2, float64(s.H)/70, w)
	cell := s.H / 22
	for row := 0; row < 4; row++ {
		for col := 0; col < 6; col++ {
			x0 := cx - s.H/8 + col*cell + row*cell/3
			y0 := cy - pole/2 + row*cell
			col2 := w
			if (row+col)%2 == 0 {
				col2 = indigoDeep
			}
			s.c.rect(x0, y0, x0+cell, y0+cell, col2)
		}
	}
}

func sceneClash(s *scene, cx, cy int) { // crossed blades
	w := hexRGB(white)
	for _, d := range [2]float64{-1, 1} {
		x0 := cx + int(d*float64(s.H)/9)
		y0 := cy - s.H/4
		x1 := cx - int(d*float64(s.H)/7)
		y1 := cy + s.H/4
		s.c.line(x0, y0, x1, y1, float64(s.H)/40, w)
		gx, gy := (x0*3+x1)/4, (y0*3+y1)/4
		s.c.line(gx-int(d)*s.H/28, gy, gx+int(d)*s.H/28, gy, float64(s.H)/55, w)
	}
	s.c.discGlow(cx, cy, float64(s.H)/26, indigoHi, 0.6)
	s.c.disc(cx, cy, float64(s.H)/70, inkWhite)
}

func splatBlob(c *canvas, cx, cy, r float64, col rgb) {
	c.disc(int(cx), int(cy), r, col)
	for i := 0; i < 7; i++ {
		a := float64(i) * 51.4 * math.Pi / 180
		rr := r * (0.45 + 0.22*float64(i%3))
		c.disc(int(cx+rr*2.1*math.Cos(a)), int(cy+rr*2.1*math.Sin(a)), r*(0.30+0.10*float64(i%2)), col)
	}
}

func sceneSplat(s *scene, cx, cy int) {
	s.c.discGlow(cx, cy, float64(s.H)/2.6, indigoMid, 0.4)
	splatBlob(s.c, float64(cx), float64(cy), float64(s.H)/5.2, indigoHi)
	splatBlob(s.c, float64(cx+s.H/3), float64(cy+s.H/5), float64(s.H)/11, inkWhite)
	splatBlob(s.c, float64(cx-s.H/3), float64(cy-s.H/6), float64(s.H)/14, indigoDeep)
}

func sceneSplatDuo(s *scene, cx, cy int) {
	s.c.discGlow(cx, cy, float64(s.H)/2.4, indigoMid, 0.4)
	splatBlob(s.c, float64(cx-s.H/6), float64(cy), float64(s.H)/4.6, indigoHi)
	splatBlob(s.c, float64(cx+s.H/6), float64(cy+4), float64(s.H)/4.6, inkWhite)
	for i := 0; i < 5; i++ {
		s.c.disc(cx-s.H/40+i%3*s.H/70, cy-s.H/9+i*s.H/22, float64(s.H)/90, indigoHi)
	}
}

func sceneIsland(s *scene, cx, cy int) { // palm + water
	hy := s.H * 62 / 100
	s.c.disc(cx+int(float64(s.W)*0.28), hy-int(s.H/4), float64(s.H)/22, hexRGB("#F5C66B"))
	s.c.blendRect(0, hy, s.W, s.H, indigoDeep, 0.35)
	for y := hy; y < s.H; y += s.H / 30 {
		s.c.blendLine(0, y, s.W, y, indigoHi, 0.15, 1)
	}
	// Island mound sitting on the horizon.
	s.c.ellipse(cx, hy+2, float64(s.H)/6, float64(s.H)/26, float64(s.H)/26, indigoHi)
	tx, ty := cx, hy-s.H/18
	s.c.line(tx, ty, tx+int(float64(s.H)/160), ty+int(float64(s.H)/9), float64(s.H)/70, inkWhite)
	// Palm fronds radiating from the crown and drooping outward.
	for _, a := range [5]float64{15, 50, 90, 130, 165} {
		s.c.arc(tx, ty-int(float64(s.H)/70), float64(s.H)/13, float64(s.H)/95, a-26, a+26, inkWhite)
	}
	s.c.disc(tx, ty-int(float64(s.H)/70), float64(s.H)/110, inkWhite)
}

func sceneSpring(s *scene, cx, cy int) { // coil + fist
	w := hexRGB(white)
	coils := 5
	for i := 0; i <= coils*24; i++ {
		t := float64(i) / float64(coils*24)
		x := cx - s.H/5 + int(t*float64(s.H)*2/5)
		y := cy + int(math.Sin(t*math.Pi*2*float64(coils))*float64(s.H)/22)
		s.c.disc(x, y, float64(s.H)/95, w)
	}
	fx, fy := cx+s.H/5+int(float64(s.H)/36), cy
	s.c.disc(fx, fy, float64(s.H)/16, w)
	s.c.disc(fx, fy, float64(s.H)/40, indigoDeep)
	s.c.disc(cx-s.H/5-int(float64(s.H)/36), cy, float64(s.H)/22, indigoHi)
}

func sceneMansion(s *scene, cx, cy int) {
	amber := hexRGB("#F5C66B")
	base := s.H * 72 / 100
	bw, bh := s.H*38/100, s.H*30/100
	s.c.rrect(cx-bw/2, base-bh, cx+bw/2, base, float64(s.H)/70, indigoDeep)
	s.c.line(cx-bw/2-int(s.H/30), base-bh, cx, base-bh-s.H*14/100, float64(s.H)/50, inkWhite)
	s.c.line(cx+bw/2+int(s.H/30), base-bh, cx, base-bh-s.H*14/100, float64(s.H)/50, inkWhite)
	for _, wx := range [3]int{cx - bw/3, cx, cx + bw/3} {
		s.c.rect(wx-s.H/50, base-bh+s.H/24, wx+s.H/50, base-bh+s.H/12, amber)
	}
	s.c.rect(cx-s.H/40, base-s.H*7/100, cx+s.H/40, base, inkWhite)
}

func sceneGolf(s *scene, cx, cy int) {
	w := hexRGB(white)
	ground := s.H * 66 / 100
	s.c.blendLine(0, ground, s.W, ground, indigoHi, 0.4, 2)
	fx := cx + s.H/8
	s.c.line(fx, ground, fx, ground-s.H/4, float64(s.H)/80, w)
	s.c.line(fx, ground-s.H/4, fx+s.H/9, ground-s.H/4+s.H/40, float64(s.H)/80, w)
	s.c.line(fx, ground-s.H/4, fx, ground-s.H/4+s.H/18, float64(s.H)/80, w)
	s.c.disc(cx-s.H/7, ground-s.H/70, float64(s.H)/50, w)
	s.c.disc(cx-s.H/7, ground-s.H/150, float64(s.H)/60, indigoDeep)
}

func sceneDice(s *scene, cx, cy int) {
	u := float64(s.H) / 364
	w := hexRGB(white)
	x0, y0 := cx-s.H/5, cy-s.H/7
	size := s.H / 4
	s.c.rrect(x0, y0, x0+size, y0+size, 24*u, w)
	for _, p := range [5][2]float64{{-1, -1}, {1, -1}, {0, 0}, {-1, 1}, {1, 1}} {
		s.c.disc(x0+size/2+int(p[0]*52*u), y0+size/2+int(p[1]*52*u), 20*u, indigoDeep)
	}
	x1, y1 := cx+s.H/16, cy+s.H/22
	size2 := s.H / 6
	s.c.rrect(x1, y1, x1+size2, y1+size2, 16*u, indigoHi)
	s.c.disc(x1+size2/2, y1+size2/2, 16*u, indigoDeep)
}

func sceneStrike(s *scene, cx, cy int) {
	bx, by := cx+s.H/16, cy
	s.c.disc(bx, by, float64(s.H)/6, indigoHi)
	for i := 0; i < 5; i++ {
		a := -math.Pi/2 + float64(i)*2*math.Pi/5
		s.c.disc(bx+int(math.Round(float64(s.H)/22*math.Cos(a))), by+int(math.Round(float64(s.H)/22*math.Sin(a))), float64(s.H)/55, indigoDeep)
	}
	for i := 0; i < 3; i++ {
		s.c.blendLine(bx-s.H/4-int(i)*s.H/26, by-s.H/16+i*s.H/16, bx-s.H/8, by-s.H/16+i*s.H/16, inkWhite, 0.35, 2)
	}
}

func sceneTennis(s *scene, cx, cy int) {
	u := float64(s.H) / 364
	w := hexRGB(white)
	rx, ry, rr := cx-s.H/24, cy, float64(s.H)/5.6
	for t := 0; t < 360; t += 2 {
		th := float64(t) * math.Pi / 180
		s.c.disc(rx+int(math.Round(rr*1.35*math.Cos(th))), ry+int(math.Round(rr*math.Sin(th))), 7*u, w)
	}
	for i := -2; i <= 2; i++ {
		s.c.blendLine(rx-int(rr*1.2), ry+i*int(rr/2.6), rx+int(rr*1.2), ry+i*int(rr/2.6), indigoHi, 0.35, 1)
	}
	for i := -3; i <= 3; i++ {
		s.c.blendLine(rx+i*int(rr/2.4), ry-int(rr*0.95), rx+i*int(rr/2.4), ry+int(rr*0.95), indigoHi, 0.35, 1)
	}
	s.c.line(rx+int(rr*1.3), ry, rx+int(rr*1.3)+s.H/6, ry+s.H/9, 22*u, w)
	s.c.disc(cx+s.H/4, cy-s.H/8, float64(s.H)/40, w)
}

func sceneClaws(s *scene, cx, cy int) {
	w := hexRGB(white)
	for i := 0; i < 3; i++ {
		x0 := cx - s.H/4 + i*s.H/11
		y0 := cy - s.H/4
		x1 := x0 + s.H/7
		y1 := cy + s.H/3
		s.c.line(x0, y0, x1, y1, float64(s.H)/26, w)
		s.c.disc(x0, y0, float64(s.H)/85, w)
	}
}

func sceneBlock(s *scene, cx, cy int) {
	amber := hexRGB("#F5C66B")
	u := float64(s.H) / 364
	bw, bh := int(88*u), int(44*u)
	for row := 0; row < 2; row++ {
		off := 0
		if row%2 == 1 {
			off = bw / 2
		}
		for col := -2; col <= 1; col++ {
			x0 := cx - bw + col*bw + off
			y0 := cy + row*bh
			s.c.rrect(x0+3, y0+3, x0+bw-3, y0+bh-3, 6*u, indigoHi)
		}
	}
	cx2, cy2 := cx+int(4*u), cy-int(70*u)
	s.c.discGlow(cx2, cy2, float64(s.H)/22, amber, 0.4)
	s.c.disc(cx2, cy2, float64(s.H)/40, amber)
	s.c.rect(cx2-int(s.H)/130, cy2-int(s.H)/60, cx2+int(s.H)/130, cy2+int(s.H)/60, indigoDeep)
}

func sceneRelay(s *scene, cx, cy int) { // Wii: signal relay between two points
	w := hexRGB(white)
	ax, ay := cx-s.H/4, cy+s.H/10
	bx, by := cx+s.H/4, cy-s.H/10
	s.c.disc(ax, ay, float64(s.H)/30, indigoHi)
	s.c.ring(bx, by, float64(s.H)/26, float64(s.H)/90, w)
	for k := 1; k <= 3; k++ {
		midx := (ax + bx) / 2
		midy := (ay+by)/2 - k*s.H/38
		s.c.arc(midx, midy+int(float64(s.H)/26), float64(s.H)*float64(3+k)/22, float64(s.H)/110, 220, 320, w)
	}
}

func sceneBeacon(s *scene, cx, cy int) { // DS: mark with signal arcs
	drawMark(s.c, cx-s.H/7, cy, s.H*2/7, nil)
	for k := 1; k <= 3; k++ {
		s.c.arc(cx+s.H/14, cy, float64(s.H)*float64(5+k*3)/44, float64(s.H)/110, 300, 420, inkWhite)
	}
}

func init() {
	// Register the extended motif set (plain names in sceneByName).
	extra := map[string]motifFunc{
		"globe": sceneGlobe, "farm": sceneFarm, "pod": scenePod,
		"flag": sceneFlag, "clash": sceneClash, "splat": sceneSplat,
		"splatduo": sceneSplatDuo, "island": sceneIsland, "spring": sceneSpring,
		"mansion": sceneMansion, "golf": sceneGolf, "dice": sceneDice,
		"strike": sceneStrike, "tennis": sceneTennis, "claws": sceneClaws,
		"block": sceneBlock, "relay": sceneRelay, "beacon": sceneBeacon,
	}
	for name, m := range extra {
		registerMotif(name, m)
	}
}

// ---------- real screenshots ----------

// loadScreenshot returns an operator-captured gameplay image for an article,
// from channels/screenshots/<news_id>.png|jpg|jpeg. Absent file = none.
func loadScreenshot(sourceDir string, id int64) image.Image {
	return loadImage(ScreenshotPath(sourceDir, id))
}

// coverCrop center-crops an image to the target aspect, then box-filter
// scales it to exactly w×h. Deterministic, no external dependencies.
func coverCrop(im image.Image, w, h int) *image.RGBA {
	b := im.Bounds()
	sw, sh := b.Dx(), b.Dy()
	cw, ch := sw, sw*h/w
	if ch > sh {
		cw, ch = sh*w/h, sh
	}
	x0 := b.Min.X + (sw-cw)/2
	y0 := b.Min.Y + (sh-ch)/2
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for py := 0; py < h; py++ {
		sy0 := y0 + py*ch/h
		sy1 := y0 + (py+1)*ch/h
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for px := 0; px < w; px++ {
			sx0 := x0 + px*cw/w
			sx1 := x0 + (px+1)*cw/w
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var rs, gs, bs, n float64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, _ := im.At(sx, sy).RGBA()
					rs += float64(cr >> 8)
					gs += float64(cg >> 8)
					bs += float64(cb >> 8)
					n++
				}
			}
			out.SetRGBA(px, py, rgb{
				R: uint8(math.Round(rs / n)), G: uint8(math.Round(gs / n)),
				B: uint8(math.Round(bs / n)), A: 255})
		}
	}
	return out
}
