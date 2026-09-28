// Command black-white-qr builds a spec-decodable QR code whose light-module
// area is as large as possible.
//
// Only the mode/count/content/terminator bits are read by decoders; every data
// bit after the terminator is free. Those free bits are chosen so that, after
// masking, as many modules as possible (data *and* error-correction) are light.
// Since Reed-Solomon is GF(2)-linear, that is a minimum-weight syndrome decoding
// problem per RS block, attacked here with an information-set local search.
package main

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/makiuchi-d/gozxing/qrcode/decoder"
)

func main() {
	versions := flag.String("versions", "1-40", "version range to consider, e.g. 6 or 1-40")
	steps := flag.Int("steps", 200000, "ECC search steps per RS block")
	out := flag.String("out", "black-white-qr", "output path without extension")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("usage: black-white-qr [flags] text")
	}
	text := flag.Arg(0)
	lo, hi, err := parseRange(*versions)
	if err != nil {
		log.Fatal(err)
	}

	type candidate struct {
		ratio         float64
		version, mask int
	}
	var survey []candidate
	for v := lo; v <= hi; v++ {
		for mask := range 8 {
			m, err := build(text, v, mask, 0, 0, false)
			if err != nil {
				break
			}
			survey = append(survey, candidate{float64(darkCount(m)) / float64(len(m)*len(m)), v, mask})
		}
	}
	if len(survey) == 0 {
		log.Fatalf("%q does not fit in versions %s", text, *versions)
	}
	slices.SortFunc(survey, func(a, b candidate) int { return cmp.Compare(a.ratio, b.ratio) })
	fmt.Println("baseline (padding light, no ECC search), best 5:")
	for _, c := range survey[:min(5, len(survey))] {
		fmt.Printf("  v%d mask%d: dark %.4f\n", c.version, c.mask, c.ratio)
	}

	version := survey[0].version
	var best [][]bool
	bestMask := 0
	for mask := range 8 {
		m, err := build(text, version, mask, *steps, 0, false)
		if err != nil {
			log.Fatal(err)
		}
		if best == nil || darkCount(m) < darkCount(best) {
			best, bestMask = m, mask
		}
	}
	if err := writePNG(best, *out+".png"); err != nil {
		log.Fatal(err)
	}
	if err := writeSVG(best, *out+".svg"); err != nil {
		log.Fatal(err)
	}
	n := len(best) * len(best)
	fmt.Printf("optimized v%d mask%d: dark %d/%d = %.4f\n", version, bestMask, darkCount(best), n, float64(darkCount(best))/float64(n))
	decoded, err := decoder.NewDecoder().DecodeBoolMapWithoutHint(best)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("decoded: %q\n", decoded.GetText())
}

func parseRange(s string) (lo, hi int, err error) {
	loS, hiS, found := strings.Cut(s, "-")
	if !found {
		hiS = loS
	}
	if _, err := fmt.Sscan(loS, &lo); err != nil {
		return 0, 0, fmt.Errorf("bad version range %q", s)
	}
	if _, err := fmt.Sscan(hiS, &hi); err != nil {
		return 0, 0, fmt.Errorf("bad version range %q", s)
	}
	if lo < 1 || hi > 40 || lo > hi {
		return 0, 0, fmt.Errorf("version range %q must lie within 1-40", s)
	}
	return lo, hi, nil
}

const border = 4

func writePNG(m [][]bool, path string) error {
	const scale = 8
	size := (len(m) + 2*border) * scale
	img := image.NewGray(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			i, j := y/scale-border, x/scale-border
			c := color.White
			if i >= 0 && j >= 0 && i < len(m) && j < len(m) && m[i][j] {
				c = color.Black
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeSVG(m [][]bool, path string) error {
	size := len(m) + 2*border
	var d strings.Builder
	for i, row := range m {
		for j, v := range row {
			if v {
				fmt.Fprintf(&d, "M%d %dh1v1h-1z", j+border, i+border)
			}
		}
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`+
		`<rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="%s"/></svg>`+"\n", size, size, d.String())
	return os.WriteFile(path, []byte(svg), 0o644)
}
