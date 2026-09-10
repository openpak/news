// SPDX-License-Identifier: AGPL-3.0-only
// Original programmatic artwork for the OpenPak channel seed. Every pixel
// is drawn here from integer math; nothing is copied from games, Nintendo
// assets or stock art. Output is deterministic for a given motif, hue and
// size, so rebuilds are reproducible.

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"os/exec"
)

type rgb = color.RGBA

var (
	navy   = rgb{R: 16, G: 26, B: 44, A: 255}
	inkCol = rgb{R: 232, G: 238, B: 244, A: 255}
)

func hsv(h float64, s, v float64) rgb {
	r, g, b := hsvToRGB(math.Remainder(h, 1.0), s, v)
	return rgb{R: r, G: g, B: b, A: 255}
}

// hsvToRGB converts hue in [0,1), saturation and value to 8-bit RGB.
func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	if h < 0 {
		h += 1
	}
	i := int(math.Floor(h*6)) % 6
	f := h*6 - math.Floor(h*6)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch i {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return uint8(math.Round(r * 255)), uint8(math.Round(g * 255)), uint8(math.Round(b * 255))
}

type palette struct {
	bgTop, bgBottom, accent, alt, ink rgb
}

func paletteFor(hue int) palette {
	h := float64(hue%360) / 360
	return palette{
		bgTop:    navy,
		bgBottom: hsv(h, 0.45, 0.24),
		accent:   hsv(h, 0.72, 0.86),
		alt:      hsv(math.Remainder(h+0.12, 1.0), 0.52, 0.60),
		ink:      inkCol,
	}
}

// canvas is a tiny deterministic raster with the few primitives the motifs
// need. Coordinates are inclusive-exclusive on the right/bottom edge.
type canvas struct {
	w, h int
	px   []rgb
}

// image.Image compliance lets encodeJPEG read the canvas generically.
func (c *canvas) At(x, y int) color.Color { return c.px[y*c.w+x] }
func (c *canvas) ColorModel() color.Model { return color.RGBAModel }
func (c *canvas) Bounds() image.Rectangle { return image.Rect(0, 0, c.w, c.h) }

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, px: make([]rgb, w*h)}
}

func (c *canvas) set(x, y int, col rgb) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.px[y*c.w+x] = col
}

func (c *canvas) gradient(top, bottom rgb) {
	for y := 0; y < c.h; y++ {
		t := float64(y) / float64(max(c.h-1, 1))
		for x := 0; x < c.w; x++ {
			c.set(x, y, lerp(top, bottom, t))
		}
	}
}

func (c *canvas) fill(col rgb) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			c.set(x, y, col)
		}
	}
}

func (c *canvas) rect(x0, y0, x1, y1 int, col rgb) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			c.set(x, y, col)
		}
	}
}

func (c *canvas) disc(cx, cy, r int, col rgb) {
	if r <= 0 {
		return
	}
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				c.set(x, y, col)
			}
		}
	}
}

func (c *canvas) ring(cx, cy, r, thick int, col rgb) {
	if r <= 0 || thick <= 0 {
		return
	}
	outer, inner := r, r-thick
	for y := cy - outer; y <= cy+outer; y++ {
		for x := cx - outer; x <= cx+outer; x++ {
			dx, dy := x-cx, y-cy
			d := dx*dx + dy*dy
			if d <= outer*outer && d > inner*inner {
				c.set(x, y, col)
			}
		}
	}
}

func (c *canvas) line(x0, y0, x1, y1, thick int, col rgb) {
	steps := max(abs(x1-x0), abs(y1-y0), 1)
	half := max(thick/2, 0)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := x0 + int(math.Round(float64(x1-x0)*t))
		y := y0 + int(math.Round(float64(y1-y0)*t))
		c.disc(x, y, half, col)
	}
}

// motif paints into a square of side m centred on (cx, cy).
type motif func(c *canvas, cx, cy, m int, p palette)

func motifByName(name string) (motif, error) {
	switch name {
	case "beacon":
		return mBeacon, nil
	case "pad":
		return mPad, nil
	case "gear":
		return mGear, nil
	case "buoy":
		return mBuoy, nil
	case "waves":
		return mWaves, nil
	case "rays":
		return mRays, nil
	case "orbit":
		return mOrbit, nil
	case "grid":
		return mGrid, nil
	case "steps":
		return mSteps, nil
	case "spark":
		return mSpark, nil
	default:
		return nil, fmt.Errorf("unknown motif %q", name)
	}
}

// mBeacon: a broadcast mark — centre dot, two signal rings and short
// diagonal ticks between them. Everything stays well inside the frame.
func mBeacon(c *canvas, cx, cy, m int, p palette) {
	c.disc(cx, cy, m/9, p.accent)
	c.ring(cx, cy, m*5/18, m/22, p.ink)
	c.ring(cx, cy, m*13/36, m/20, p.accent)
	for _, d := range [4][2]int{{1, -1}, {1, 1}, {-1, 1}, {-1, -1}} {
		x0 := cx + d[0]*m*7/36
		y0 := cy + d[1]*m*7/36
		x1 := cx + d[0]*m*11/36
		y1 := cy + d[1]*m*11/36
		c.line(x0, y0, x1, y1, m/18, p.ink)
	}
}

// mPad: an abstract controller — a directional cross on the left and two
// round buttons on the right.
func mPad(c *canvas, cx, cy, m int, p palette) {
	px := cx - m*3/16
	arm, thick := m/5, m/14
	c.rect(px-arm, cy-thick, px+arm, cy+thick, p.accent) // cross bar
	c.rect(px-thick, cy-arm, px+thick, cy+arm, p.accent) // upright
	c.disc(px, cy, thick/2+1, navy)                      // hub dimple
	bx := cx + m*9/32
	c.disc(bx, cy-m/10, m/11, p.ink)
	c.disc(bx+m/6, cy+m/10, m/11, p.alt)
}

// mGear: hexagon and hub, the workshop mark.
func mGear(c *canvas, cx, cy, m int, p palette) {
	r := m * 8 / 25
	pts := make([][2]int, 6)
	for i := range pts {
		a := float64(i) * math.Pi / 3
		pts[i] = [2]int{cx + int(math.Round(float64(r)*math.Cos(a))),
			cy + int(math.Round(float64(r)*math.Sin(a)))}
	}
	for i := range pts {
		j := (i + 1) % len(pts)
		c.line(pts[i][0], pts[i][1], pts[j][0], pts[j][1], m/13, p.accent)
	}
	c.disc(cx, cy, m/7, p.alt)
	c.disc(cx, cy, m/16, navy)
	for i := 0; i < 6; i++ {
		a := float64(i) * math.Pi / 3
		c.line(cx+int(math.Round(float64(r)*0.72*math.Cos(a))),
			cy+int(math.Round(float64(r)*0.72*math.Sin(a))),
			cx+int(math.Round(float64(r)*1.5*math.Cos(a))),
			cy+int(math.Round(float64(r)*1.5*math.Sin(a))), m/16, p.ink)
	}
}

// mBuoy: a lifebuoy ring with four gaps — the help mark.
func mBuoy(c *canvas, cx, cy, m int, p palette) {
	r, thick := m*8/25, m*9/64
	c.ring(cx, cy, r, thick, p.accent)
	g := m / 11
	c.rect(cx-g, cy-r-thick, cx+g, cy-r+thick, navy) // north gap
	c.rect(cx-g, cy+r-thick, cx+g, cy+r+thick, navy) // south gap
	c.rect(cx-r-thick, cy-g, cx-r+thick, cy+g, navy) // west gap
	c.rect(cx+r-thick, cy-g, cx+r+thick, cy+g, navy) // east gap
	c.disc(cx, cy, m/9, p.alt)
}

// mWaves: layered sine bands.
func mWaves(c *canvas, cx, cy, m int, p palette) {
	span := m * 9 / 10
	for i := 0; i < 4; i++ {
		col := p.accent
		if i%2 == 1 {
			col = p.alt
		}
		for dx := -span; dx <= span; dx++ {
			x := cx + dx
			y := cy + (i-2)*m*3/20 + int(math.Round(float64(dx)/float64(m)*3.2*math.Sin(float64(dx)*0.09)))
			c.disc(x, y, m/22, col)
		}
	}
}

// mRays: diagonal stripes rising to the upper right.
func mRays(c *canvas, cx, cy, m int, p palette) {
	span := m * 3 / 5
	for k := -5; k <= 5; k++ {
		col := p.accent
		if k%2 == 0 {
			col = p.alt
		}
		x0 := cx + k*span/2
		c.line(x0, cy+span, x0+span*2/3, cy-span, m/16, col)
	}
	c.disc(cx, cy, m/9, p.ink)
}

// mOrbit: a ring, two bodies and a centre star.
func mOrbit(c *canvas, cx, cy, m int, p palette) {
	c.ring(cx, cy, m*7/20, m/18, p.alt)
	a := -math.Pi / 5
	px := cx + int(math.Round(float64(m)*7/20*math.Cos(a)))
	py := cy + int(math.Round(float64(m)*7/20*math.Sin(a)))
	c.disc(px, py, m/9, p.accent)
	c.disc(cx-m*7/20, cy, m/14, p.ink)
	c.disc(cx, cy, m/16, p.ink)
}

// mGrid: a dot lattice with a highlighted sine path.
func mGrid(c *canvas, cx, cy, m int, p palette) {
	step := m / 5
	for gy := -2; gy <= 2; gy++ {
		for gx := -2; gx <= 2; gx++ {
			x, y := cx+gx*step, cy+gy*step
			col := p.alt
			if gy == 0 && gx >= -2 {
				col = p.accent
				y = cy + int(math.Round(float64(gx)*math.Sin(float64(gx)*0.9)*float64(step)/2.4))
			}
			c.disc(x, y, m/16, col)
		}
	}
}

// mSteps: ascending bars with a milestone dot on top.
func mSteps(c *canvas, cx, cy, m int, p palette) {
	base := cy + m*2/5
	w := m / 5
	for i := 0; i < 4; i++ {
		hgt := m * (i + 2) / 8
		x0 := cx - m*2/5 + i*w*9/8
		col := p.accent
		if i%2 == 1 {
			col = p.alt
		}
		c.rect(x0, base-hgt, x0+w, base, col)
		if i == 3 {
			c.disc(x0+w/2, base-hgt-m/14, m/16, p.ink)
		}
	}
}

// mSpark: a centre plus and a deterministic scatter of small ones.
func mSpark(c *canvas, cx, cy, m int, p palette) {
	arm := m / 12
	for gy := -2; gy <= 2; gy++ {
		for gx := -2; gx <= 2; gx++ {
			if (gx*5+gy*3+100)%4 != 0 || (gx == 0 && gy == 0) {
				continue
			}
			x := cx + gx*m*2/7
			y := cy + gy*m*2/7
			col := p.alt
			if (gx*7+gy*11+100)%3 == 0 {
				col = p.ink
			}
			c.rect(x-arm/2, y-arm/2, x+arm/2+1, y+arm/2+1, col)
		}
	}
	c.rect(cx-arm*3/2, cy-arm/2, cx+arm*3/2, cy+arm/2, p.accent)
	c.rect(cx-arm/2, cy-arm*3/2, cx+arm/2, cy+arm*3/2, p.accent)
}

// renderIcon paints a 70x70 channel icon: solid navy, motif, thin frame.
func renderIcon(name string, hue int) (image.Image, error) {
	m, err := motifByName(name)
	if err != nil {
		return nil, err
	}
	c := newCanvas(70, 70)
	p := paletteFor(hue)
	c.fill(navy)
	m(c, 35, 35, 58, p)
	c.rect(0, 0, 70, 2, p.accent)
	c.rect(0, 68, 70, 70, p.accent)
	return c, nil
}

// renderArt paints a card or hero: gradient background, centred motif and
// a footer bar.
func renderArt(name string, hue, w, h int) (image.Image, error) {
	m, err := motifByName(name)
	if err != nil {
		return nil, err
	}
	p := paletteFor(hue)
	c := newCanvas(w, h)
	c.gradient(p.bgTop, p.bgBottom)
	m(c, w/2, h/2, h, p)
	c.rect(0, h-h/36, w, h, p.accent)
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
	cmd := exec.Command("cjpeg", "-quality", "85", "-baseline", "-sample", "1x1")
	cmd.Stdin = &ppm
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cjpeg (libjpeg-turbo) is required: %w", err)
	}
	return out, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func lerp(a, b rgb, t float64) rgb {
	mix := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x)*(1-t) + float64(y)*t))
	}
	return rgb{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 255}
}
