package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

type block struct {
	numData, numECC int
	dataIndex       []int // final-message bit index of each data bit
	eccIndex        []int // final-message bit index of each ECC bit
}

// layout interleaves the RS blocks into the final message (ISO/IEC 18004 7.6).
func layout(version int) []block {
	var blocks []block
	for _, g := range blocksL(version) {
		for range g.count {
			blocks = append(blocks, block{numData: g.data, numECC: g.ecc})
		}
	}
	cw := 0
	for idx := range blocks[len(blocks)-1].numData {
		for b := range blocks {
			if idx < blocks[b].numData {
				blocks[b].dataIndex = appendByte(blocks[b].dataIndex, cw)
				cw++
			}
		}
	}
	for range blocks[0].numECC {
		for b := range blocks {
			blocks[b].eccIndex = appendByte(blocks[b].eccIndex, cw)
			cw++
		}
	}
	return blocks
}

func appendByte(index []int, codeword int) []int {
	for t := range 8 {
		index = append(index, 8*codeword+t)
	}
	return index
}

func toBits(bs []byte) []bool {
	out := make([]bool, 0, 8*len(bs))
	for _, b := range bs {
		for t := 7; t >= 0; t-- {
			out = append(out, b>>t&1 == 1)
		}
	}
	return out
}

func fromBits(bits []bool) []byte {
	out := make([]byte, len(bits)/8)
	for i, b := range bits {
		if b {
			out[i/8] |= 0x80 >> (i % 8)
		}
	}
	return out
}

// eccColumns returns, for each data bit p, the ECC bits produced by data bit p alone.
func eccColumns(numData, numECC int) []vec {
	cols := make([]vec, 8*numData)
	for p := range cols {
		data := make([]byte, numData)
		data[p/8] = 0x80 >> (p % 8)
		for q, b := range toBits(rsECC(data, numECC)) {
			if b {
				cols[p] = cols[p].xor(unit(q))
			}
		}
	}
	return cols
}

// build returns the module matrix (true = dark). With standardPadding it emits an
// ordinary QR code (0xEC/0x11 pad codewords); otherwise every bit after the
// terminator is chosen to minimise dark modules, searching the ECC for `steps` steps.
func build(text string, version, mask, steps int, seed uint64, standardPadding bool) ([][]bool, error) {
	header, err := headerBits(text, version)
	if err != nil {
		return nil, err
	}
	stream := header
	if standardPadding {
		stream = padStream(header, dataCapacityBits(version))
	}
	pattern := functionPattern(version)
	positions := codewordPositions(pattern)
	maskBits := make([]bool, len(positions))
	for n, p := range positions {
		maskBits[n] = maskBit(mask, p[0], p[1])
	}
	// Remainder bits are never read: 0 as the spec says, or light otherwise.
	message := make([]bool, len(positions))
	if !standardPadding {
		copy(message, maskBits)
	}

	rng := rand.New(rand.NewPCG(seed, 0))
	columns := map[[2]int][]vec{}
	offset := 0
	for _, blk := range layout(version) {
		own := stream[min(offset, len(stream)):min(offset+8*blk.numData, len(stream))]
		offset += 8 * blk.numData
		key := [2]int{blk.numData, blk.numECC}
		if columns[key] == nil {
			columns[key] = eccColumns(blk.numData, blk.numECC)
		}
		fillBlock(message, maskBits, blk, own, columns[key], steps, rng)
		if !eccConsistent(message, blk) {
			return nil, fmt.Errorf("%w: ECC of v%d mask %d is inconsistent", errInternal, version, mask)
		}
	}

	m := make([][]bool, len(pattern))
	for i := range m {
		m[i] = make([]bool, len(pattern))
		for j := range m[i] {
			m[i][j] = pattern[i][j] == moduleDark
		}
	}
	for k, p := range positions {
		m[p[0]][p[1]] = message[k] != maskBits[k]
	}
	addFormatAndVersion(m, version, mask)
	return m, nil
}

// fillBlock writes one RS block into message: the fixed bits `own` first, then
// the free data bits and the ECC bits, chosen so that few modules end up dark.
func fillBlock(message, maskBits []bool, blk block, own []bool, dataCols []vec, steps int, rng *rand.Rand) {
	var cols []vec
	var index []int
	for p := len(own); p < 8*blk.numData; p++ {
		cols = append(cols, dataCols[p])
		index = append(index, blk.dataIndex[p])
	}
	for q := range 8 * blk.numECC {
		cols = append(cols, unit(q))
	}
	index = append(index, blk.eccIndex...)

	var syndrome vec
	for p, b := range own {
		if b {
			syndrome = syndrome.xor(dataCols[p])
		}
	}
	for n, c := range cols {
		if maskBits[index[n]] {
			syndrome = syndrome.xor(c)
		}
	}
	dark := minWeightSolution(cols, 8*blk.numECC, syndrome, steps, rng)

	for p, b := range own {
		message[blk.dataIndex[p]] = b
	}
	for n, idx := range index {
		message[idx] = maskBits[idx] != dark[n]
	}
}

func eccConsistent(message []bool, blk block) bool {
	data := make([]bool, len(blk.dataIndex))
	for p, idx := range blk.dataIndex {
		data[p] = message[idx]
	}
	ecc := make([]bool, len(blk.eccIndex))
	for q, idx := range blk.eccIndex {
		ecc[q] = message[idx]
	}
	return slices.Equal(toBits(rsECC(fromBits(data), blk.numECC)), ecc)
}

// padStream appends padding bits and the 0xEC/0x11 pad codewords (ISO/IEC 18004 7.4.10).
func padStream(header []bool, capacity int) []bool {
	s := slices.Clone(header)
	for len(s)%8 != 0 {
		s = append(s, false)
	}
	for k := 0; len(s) < capacity; k++ {
		s = append(s, toBits([]byte{[]byte{0xec, 0x11}[k%2]})...)
	}
	return s
}

func darkCount(m [][]bool) int {
	n := 0
	for _, row := range m {
		for _, v := range row {
			if v {
				n++
			}
		}
	}
	return n
}
