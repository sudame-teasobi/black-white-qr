package main

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

var (
	errNotUTF8  = errors.New("text is not valid UTF-8")
	errTooLong  = errors.New("text does not fit")
	errInternal = errors.New("internal error")
)

// blockGroup is one row of ISO/IEC 18004 Table 9 for error correction level L.
type blockGroup struct{ count, data, ecc int }

func blocksL(version int) []blockGroup {
	return [...][]blockGroup{
		nil,
		{{1, 19, 7}},
		{{1, 34, 10}},
		{{1, 55, 15}},
		{{1, 80, 20}},
		{{1, 108, 26}},
		{{2, 68, 18}},
		{{2, 78, 20}},
		{{2, 97, 24}},
		{{2, 116, 30}},
		{{2, 68, 18}, {2, 69, 18}},
		{{4, 81, 20}},
		{{2, 92, 24}, {2, 93, 24}},
		{{4, 107, 26}},
		{{3, 115, 30}, {1, 116, 30}},
		{{5, 87, 22}, {1, 88, 22}},
		{{5, 98, 24}, {1, 99, 24}},
		{{1, 107, 28}, {5, 108, 28}},
		{{5, 120, 30}, {1, 121, 30}},
		{{3, 113, 28}, {4, 114, 28}},
		{{3, 107, 28}, {5, 108, 28}},
		{{4, 116, 28}, {4, 117, 28}},
		{{2, 111, 28}, {7, 112, 28}},
		{{4, 121, 30}, {5, 122, 30}},
		{{6, 117, 30}, {4, 118, 30}},
		{{8, 106, 26}, {4, 107, 26}},
		{{10, 114, 28}, {2, 115, 28}},
		{{8, 122, 30}, {4, 123, 30}},
		{{3, 117, 30}, {10, 118, 30}},
		{{7, 116, 30}, {7, 117, 30}},
		{{5, 115, 30}, {10, 116, 30}},
		{{13, 115, 30}, {3, 116, 30}},
		{{17, 115, 30}},
		{{17, 115, 30}, {1, 116, 30}},
		{{13, 115, 30}, {6, 116, 30}},
		{{12, 121, 30}, {7, 122, 30}},
		{{6, 121, 30}, {14, 122, 30}},
		{{17, 122, 30}, {4, 123, 30}},
		{{4, 122, 30}, {18, 123, 30}},
		{{20, 117, 30}, {4, 118, 30}},
		{{19, 118, 30}, {6, 119, 30}},
	}[version]
}

func symbolSize(version int) int { return 17 + 4*version }

// alignmentPositions returns the row/column centres of alignment patterns
// (ISO/IEC 18004 Annex E): 6, then evenly spaced up to size-7, with an even step.
func alignmentPositions(version int) []int {
	if version == 1 {
		return nil
	}
	n := version/7 + 2
	step := (version*8 + n*3 + 5) / (n*4 - 4) * 2
	pos := make([]int, n)
	pos[0] = 6
	for i, p := n-1, symbolSize(version)-7; i >= 1; i, p = i-1, p-step {
		pos[i] = p
	}
	return pos
}

func dataCapacityBits(version int) int {
	total := 0
	for _, g := range blocksL(version) {
		total += g.count * g.data
	}
	return 8 * total
}

type module uint8

const (
	moduleData module = iota
	moduleLight
	moduleDark
)

type grid [][]module

func (m grid) set(i, j int, dark bool) {
	if dark {
		m[i][j] = moduleDark
	} else {
		m[i][j] = moduleLight
	}
}

func (m grid) reserve(i, j int) {
	if m[i][j] == moduleData {
		m[i][j] = moduleLight
	}
}

// functionPattern marks every non-data module; data modules stay moduleData.
func functionPattern(version int) grid {
	n := symbolSize(version)
	m := make(grid, n)
	for i := range m {
		m[i] = make([]module, n)
	}
	for _, corner := range [][2]int{{0, 0}, {0, n - 7}, {n - 7, 0}} {
		m.addFinder(corner[0], corner[1])
	}
	for k := 8; k < n-8; k++ {
		m.set(6, k, k%2 == 0)
		m.set(k, 6, k%2 == 0)
	}
	m.addAlignments(alignmentPositions(version))
	m.reserveFormatAndVersion(version)
	return m
}

// addFinder draws a finder pattern with its separator, top-left corner at (top, left).
func (m grid) addFinder(top, left int) {
	n := len(m)
	for di := -1; di <= 7; di++ {
		for dj := -1; dj <= 7; dj++ {
			i, j := top+di, left+dj
			if i >= 0 && j >= 0 && i < n && j < n {
				d := max(abs(di-3), abs(dj-3))
				m.set(i, j, d != 2 && d != 4)
			}
		}
	}
}

func (m grid) addAlignments(pos []int) {
	for _, r := range pos {
		for _, c := range pos {
			last := pos[len(pos)-1]
			if (r == 6 && c == 6) || (r == 6 && c == last) || (r == last && c == 6) {
				continue // overlaps a finder pattern
			}
			for di := -2; di <= 2; di++ {
				for dj := -2; dj <= 2; dj++ {
					m.set(r+di, c+dj, max(abs(di), abs(dj)) != 1)
				}
			}
		}
	}
}

// reserveFormatAndVersion keeps the format/version areas out of the data region
// and places the always-dark module.
func (m grid) reserveFormatAndVersion(version int) {
	n := len(m)
	for i := range 9 {
		m.reserve(i, 8)
		m.reserve(8, i)
	}
	for i := range 8 {
		m.reserve(n-1-i, 8)
		m.reserve(8, n-1-i)
	}
	m.set(n-8, 8, true)
	if version < 7 {
		return
	}
	for i := range 6 {
		for j := range 3 {
			m.reserve(i, n-11+j)
			m.reserve(n-11+j, i)
		}
	}
}

// codewordPositions lists data-region modules in placement order (ISO/IEC 18004 7.7.3):
// two-module columns from the right, alternately upwards and downwards, skipping column 6.
func codewordPositions(m grid) [][2]int {
	n := len(m)
	var pos [][2]int
	upwards := true
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for v := range n {
			i := v
			if upwards {
				i = n - 1 - v
			}
			for z := range 2 {
				if j := right - z; m[i][j] == moduleData {
					pos = append(pos, [2]int{i, j})
				}
			}
		}
		upwards = !upwards
	}
	return pos
}

func maskBit(mask, i, j int) bool {
	switch mask {
	case 0:
		return (i+j)%2 == 0
	case 1:
		return i%2 == 0
	case 2:
		return j%3 == 0
	case 3:
		return (i+j)%3 == 0
	case 4:
		return (i/2+j/3)%2 == 0
	case 5:
		return i*j%2+i*j%3 == 0
	case 6:
		return (i*j%2+i*j%3)%2 == 0
	default:
		return ((i+j)%2+i*j%3)%2 == 0
	}
}

// bch appends the remainder of data·x^(degree of gen) divided by gen.
func bch(data, gen int) int {
	deg := 0
	for g := gen; g > 1; g >>= 1 {
		deg++
	}
	v := data << deg
	for i := deg + bitLen(data) - 1; i >= deg; i-- {
		if v>>i&1 == 1 {
			v ^= gen << (i - deg)
		}
	}
	return data<<deg | v
}

func formatInfo(mask int) int {
	const levelL = 0b01
	return bch(levelL<<3|mask, 0x537) ^ 0x5412
}

func versionInfo(version int) int { return bch(version, 0x1f25) }

func addFormatAndVersion(m [][]bool, version, mask int) {
	n := len(m)
	fi := formatInfo(mask)
	skip := 0
	for i := range 8 {
		if i == 6 {
			skip = 1 // the timing pattern sits at index 6
		}
		lo, hi := fi>>i&1 == 1, fi>>(14-i)&1 == 1
		m[i+skip][8] = lo
		m[8][i+skip] = hi
		m[8][n-1-i] = lo
		m[n-1-i][8] = hi
	}
	m[n-8][8] = true
	if version < 7 {
		return
	}
	vi := versionInfo(version)
	for i := range 6 {
		for j := range 3 {
			bit := vi>>(3*i+j)&1 == 1
			m[n-11+j][i] = bit
			m[i][n-11+j] = bit
		}
	}
}

// headerBits encodes text as one UTF-8 byte-mode segment plus terminator.
func headerBits(text string, version int) ([]bool, error) {
	if !utf8.ValidString(text) {
		return nil, errNotUTF8
	}
	var bits []bool
	put := func(v, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, v>>i&1 == 1)
		}
	}
	countBits := 8
	if version >= 10 {
		countBits = 16
	}
	put(0b0100, 4)
	put(len(text), countBits)
	for _, b := range []byte(text) {
		put(int(b), 8)
	}
	capacity := dataCapacityBits(version)
	if len(bits) > capacity || len(text) >= 1<<countBits {
		return nil, fmt.Errorf("%w: %q in version %d", errTooLong, text, version)
	}
	put(0, min(4, capacity-len(bits)))
	return bits, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func bitLen(x int) int {
	n := 0
	for ; x > 0; x >>= 1 {
		n++
	}
	return n
}
