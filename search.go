package main

import (
	"math/bits"
	"math/rand/v2"
)

// vec is a GF(2) vector over the ECC bits of one RS block (at most 30 codewords = 240 bits).
type vec [4]uint64

func unit(r int) (v vec) {
	v[r/64] = 1 << (r % 64)
	return v
}

func (a vec) xor(b vec) vec    { return vec{a[0] ^ b[0], a[1] ^ b[1], a[2] ^ b[2], a[3] ^ b[3]} }
func (a vec) and(b vec) vec    { return vec{a[0] & b[0], a[1] & b[1], a[2] & b[2], a[3] & b[3]} }
func (a vec) andNot(b vec) vec { return vec{a[0] &^ b[0], a[1] &^ b[1], a[2] &^ b[2], a[3] &^ b[3]} }
func (a vec) has(r int) bool   { return a[r/64]>>(r%64)&1 == 1 }
func (a vec) isZero() bool     { return a == vec{} }

func (a vec) count() int {
	return bits.OnesCount64(a[0]) + bits.OnesCount64(a[1]) + bits.OnesCount64(a[2]) + bits.OnesCount64(a[3])
}

func (a vec) ones() []int {
	var rows []int
	for w, x := range a {
		for ; x != 0; x &= x - 1 {
			rows = append(rows, 64*w+bits.TrailingZeros64(x))
		}
	}
	return rows
}

// minWeightSolution looks for y with XOR of cols[j] over y_j=1 equal to syndrome and few ones.
//
// cols[numData+q] must be unit(q), so the ECC bits form the starting information set:
// "every free data bit light, ECC bits whatever they have to be". Each step swaps one
// column into the basis, greedily when that lowers the weight and at random on plateaus.
func minWeightSolution(cols []vec, numRows int, syndrome vec, steps int, rng *rand.Rand) []bool {
	coords := append([]vec(nil), cols...)
	basis := make([]int, numRows)
	inBasis := make([]bool, len(cols))
	for r := range basis {
		basis[r] = len(cols) - numRows + r
		inBasis[basis[r]] = true
	}
	s := syndrome
	bestS, bestBasis := s, append([]int(nil), basis...)

	pivot := func(j, r int) {
		delta := coords[j].andNot(unit(r))
		for n := range coords {
			if coords[n].has(r) {
				coords[n] = coords[n].xor(delta)
			}
		}
		if s.has(r) {
			s = s.xor(delta)
		}
		inBasis[basis[r]] = false
		basis[r] = j
		inBasis[j] = true
	}

	var nonbasis []int
	for range steps {
		nonbasis = nonbasis[:0]
		for j := range coords {
			if !inBasis[j] {
				nonbasis = append(nonbasis, j)
			}
		}
		if len(nonbasis) == 0 {
			break
		}
		bestJ, bestW := -1, s.count()
		for _, j := range nonbasis {
			if c := coords[j]; !c.and(s).isZero() {
				if w := s.xor(c).count() + 1; w < bestW {
					bestJ, bestW = j, w
				}
			}
		}
		if bestJ >= 0 {
			rows := coords[bestJ].and(s).ones()
			pivot(bestJ, rows[len(rows)-1])
		} else {
			j := nonbasis[rng.IntN(len(nonbasis))]
			if rows := coords[j].andNot(s).ones(); len(rows) > 0 {
				pivot(j, rows[rng.IntN(len(rows))])
			}
		}
		if s.count() < bestS.count() {
			bestS, bestBasis = s, append(bestBasis[:0], basis...)
		}
	}
	y := make([]bool, len(cols))
	for _, r := range bestS.ones() {
		y[bestBasis[r]] = true
	}
	return y
}
