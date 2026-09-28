package main

// gf256 holds exp/log tables for GF(256) with the QR primitive polynomial x^8+x^4+x^3+x^2+1.
type gf256 struct {
	exp [512]byte
	log [256]int
}

func newGF256() *gf256 {
	g := &gf256{}
	x := byte(1)
	for i := range 255 {
		g.exp[i] = x
		g.exp[i+255] = x
		g.log[x] = i
		carry := x&0x80 != 0
		x <<= 1
		if carry {
			x ^= 0x1d
		}
	}
	return g
}

func (g *gf256) mul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return g.exp[g.log[a]+g.log[b]]
}

// generatorPoly returns (x-α^0)(x-α^1)…(x-α^(n-1)), highest degree first.
func (g *gf256) generatorPoly(n int) []byte {
	poly := []byte{1}
	for i := range n {
		next := make([]byte, len(poly)+1)
		for k, c := range poly {
			next[k] ^= c
			next[k+1] ^= g.mul(c, g.exp[i])
		}
		poly = next
	}
	return poly
}

func rsECC(data []byte, n int) []byte {
	g := newGF256()
	gen := g.generatorPoly(n)
	rem := make([]byte, n)
	for _, d := range data {
		factor := d ^ rem[0]
		copy(rem, rem[1:])
		rem[n-1] = 0
		for k := range n {
			rem[k] ^= g.mul(gen[k+1], factor)
		}
	}
	return rem
}
