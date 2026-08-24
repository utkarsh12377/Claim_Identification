package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"path"
	"path/filepath"

	"github.com/utkarsh/claim-identification/internal/model"
)

func main() {
	seedFile := flag.String("seed", filepath.Join("seed", "product.json"), "product document to read media names from")
	outDir := flag.String("out", filepath.Join("assets", "images"), "directory to write images into")
	size := flag.Int("size", 900, "image edge length in pixels")
	flag.Parse()

	if err := run(*seedFile, *outDir, *size); err != nil {
		fmt.Fprintln(os.Stderr, "genmedia:", err)
		os.Exit(1)
	}
}

func run(seedFile, outDir string, size int) error {
	raw, err := os.ReadFile(seedFile)
	if err != nil {
		return fmt.Errorf("read seed file: %w", err)
	}
	product, err := model.ParseProductDocument(raw)
	if err != nil {
		return fmt.Errorf("parse seed file: %w", err)
	}
	if len(product.Media) == 0 {
		return fmt.Errorf("product %s has no media entries", product.ID)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	for i, m := range product.Media {
		name := path.Base(m.Path)
		if name == "" || name == "." || name == "/" {
			name = fmt.Sprintf("%s-%d.jpg", product.ID, i)
		}
		dest := filepath.Join(outDir, name)

		if err := writeJPEG(dest, placeholder(size, i), 88); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", dest)
	}
	return nil
}

func placeholder(size, index int) image.Image {
	var (
		background = color.RGBA{R: 0xEE, G: 0xF1, B: 0xF4, A: 0xFF}
		frame      = color.RGBA{R: 0xC2, G: 0xCA, B: 0xD3, A: 0xFF}
		panel      = color.RGBA{R: 0xE1, G: 0xE7, B: 0xED, A: 0xFF}
		glyph      = color.RGBA{R: 0x8C, G: 0x99, B: 0xA8, A: 0xFF}
	)
	if index%2 == 1 {
		background = color.RGBA{R: 0xF2, G: 0xF0, B: 0xEA, A: 0xFF}
		panel = color.RGBA{R: 0xE8, G: 0xE4, B: 0xD9, A: 0xFF}
		glyph = color.RGBA{R: 0x9A, G: 0x92, B: 0x80, A: 0xFF}
	}

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)

	margin := size / 10
	inner := image.Rect(margin, margin, size-margin, size-margin)
	fillRect(img, inner, panel)
	strokeRect(img, inner, frame, size/120+1)

	fillCircle(img, image.Point{X: inner.Min.X + inner.Dx()/4, Y: inner.Min.Y + inner.Dy()/3}, inner.Dx()/12, glyph)

	floor := inner.Max.Y - inner.Dy()/6
	fillTriangle(img,
		image.Point{X: inner.Min.X + inner.Dx()/8, Y: floor},
		image.Point{X: inner.Min.X + inner.Dx()/2, Y: inner.Min.Y + inner.Dy()/3},
		image.Point{X: inner.Min.X + inner.Dx()*7/8, Y: floor},
		glyph)
	fillTriangle(img,
		image.Point{X: inner.Min.X + inner.Dx()/2, Y: floor},
		image.Point{X: inner.Min.X + inner.Dx()*3/4, Y: inner.Min.Y + inner.Dy()/2},
		image.Point{X: inner.Max.X - inner.Dx()/16, Y: floor},
		glyph)

	fillRect(img, image.Rect(inner.Min.X, floor, inner.Max.X, floor+size/120+1), frame)

	return img
}

func writeJPEG(dest string, img image.Image, quality int) error {
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		f.Close()
		return fmt.Errorf("encode %s: %w", dest, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dest, err)
	}
	return nil
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func strokeRect(img *image.RGBA, r image.Rectangle, c color.Color, width int) {
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+width), c)
	fillRect(img, image.Rect(r.Min.X, r.Max.Y-width, r.Max.X, r.Max.Y), c)
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+width, r.Max.Y), c)
	fillRect(img, image.Rect(r.Max.X-width, r.Min.Y, r.Max.X, r.Max.Y), c)
}

func fillCircle(img *image.RGBA, centre image.Point, radius int, c color.Color) {
	for y := centre.Y - radius; y <= centre.Y+radius; y++ {
		for x := centre.X - radius; x <= centre.X+radius; x++ {
			dx, dy := x-centre.X, y-centre.Y
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}
}

func fillTriangle(img *image.RGBA, a, b, cpt image.Point, col color.Color) {
	minX := min(a.X, min(b.X, cpt.X))
	maxX := max(a.X, max(b.X, cpt.X))
	minY := min(a.Y, min(b.Y, cpt.Y))
	maxY := max(a.Y, max(b.Y, cpt.Y))

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			p := image.Point{X: x, Y: y}
			d1 := cross(p, a, b)
			d2 := cross(p, b, cpt)
			d3 := cross(p, cpt, a)

			hasNeg := d1 < 0 || d2 < 0 || d3 < 0
			hasPos := d1 > 0 || d2 > 0 || d3 > 0
			if !(hasNeg && hasPos) {
				img.Set(x, y, col)
			}
		}
	}
}

func cross(p, a, b image.Point) int {
	return (p.X-b.X)*(a.Y-b.Y) - (a.X-b.X)*(p.Y-b.Y)
}
