package main

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/makiuchi-d/gozxing/qrcode/decoder"
	"github.com/makiuchi-d/gozxing/qrcode/encoder"
)

func TestSpecValues(t *testing.T) {
	// ISO/IEC 18004 Annex C, D and E.
	if got := formatInfo(0); got != 0b111011111000100 {
		t.Errorf("formatInfo(L, mask 0) = %015b", got)
	}
	if got := versionInfo(7); got != 0b000111110010010100 {
		t.Errorf("versionInfo(7) = %018b", got)
	}
	for v, want := range map[int][]int{7: {6, 22, 38}, 32: {6, 34, 60, 86, 112, 138}, 36: {6, 24, 50, 76, 102, 128, 154}} {
		if got := alignmentPositions(v); !slices.Equal(got, want) {
			t.Errorf("alignmentPositions(%d) = %v, want %v", v, got, want)
		}
	}
	// Annex I: "01234567" at 1-M.
	data := []byte{0x10, 0x20, 0x0c, 0x56, 0x61, 0x80, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11, 0xec, 0x11}
	want := []byte{0xa5, 0x24, 0xd4, 0xc1, 0xed, 0x36, 0xc7, 0x87, 0x2c, 0x55}
	if got := rsECC(data, 10); !bytes.Equal(got, want) {
		t.Errorf("rsECC = % x, want % x", got, want)
	}
}

func TestBlockTableMatchesModuleCount(t *testing.T) {
	for v := 1; v <= 40; v++ {
		total := 0
		for _, g := range blocksL(v) {
			total += g.count * (g.data + g.ecc)
		}
		if modules := len(codewordPositions(functionPattern(v))); modules/8 != total {
			t.Errorf("v%d: table has %d codewords, symbol has room for %d", v, total, modules/8)
		}
	}
}

func TestStandardPaddingMatchesGozxingEncoder(t *testing.T) {
	const text = "black & white"
	for v := 1; v <= 40; v++ {
		mask := v % 8
		ref, refErr := encoder.Encoder_encode(text, decoder.ErrorCorrectionLevel_L, map[gozxing.EncodeHintType]any{
			gozxing.EncodeHintType_QR_VERSION:      v,
			gozxing.EncodeHintType_QR_MASK_PATTERN: mask,
		})
		if refErr != nil {
			t.Fatal(refErr)
		}
		m, err := build(text, v, mask, 0, 0, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := range m {
			for j := range m[i] {
				if want := ref.GetMatrix().Get(j, i) == 1; m[i][j] != want {
					t.Fatalf("v%d mask %d: module (%d, %d) = %v, want %v", v, mask, i, j, m[i][j], want)
				}
			}
		}
	}
}

func TestLightPaddingDecodesWithoutErrors(t *testing.T) {
	for v := 1; v <= 40; v++ {
		for mask := range 8 {
			t.Run(fmt.Sprintf("v%d_mask%d", v, mask), func(t *testing.T) {
				m, err := build("黒", v, mask, 20, 0, false)
				if err != nil {
					t.Fatal(err)
				}
				res, err := decoder.NewDecoder().DecodeBoolMapWithoutHint(m)
				if err != nil {
					t.Fatal(err)
				}
				got := [3]any{res.GetText(), res.GetErrorsCorrected(), res.GetByteSegments()}
				want := [3]any{"黒", 0, [][]byte{[]byte("黒")}}
				if fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("decoded %v, want %v", got, want)
				}
			})
		}
	}
}

func TestSearchReducesDarkModules(t *testing.T) {
	base, err := build("黒", 6, 6, 0, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	opt, err := build("黒", 6, 6, 1000, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if darkCount(opt) >= darkCount(base) {
		t.Errorf("search gave %d dark modules, baseline %d", darkCount(opt), darkCount(base))
	}
}

func TestWrittenPNGDecodes(t *testing.T) {
	m, err := build("黒", 6, 6, 1000, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "qr.png")
	if err := writePNG(m, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		t.Fatal(err)
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, map[gozxing.DecodeHintType]any{gozxing.DecodeHintType_PURE_BARCODE: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetText() != "黒" {
		t.Errorf("decoded %q", res.GetText())
	}
}
