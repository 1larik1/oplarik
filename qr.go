package main

// Minimal QR Code encoder (ISO/IEC 18004): byte mode, error correction
// level M, versions 1–10 (up to 213 bytes). Used to show the authenticator
// setup code; no external dependencies.

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
)

type qrBlocks struct{ ec, g1, d1, g2, d2 int } // ec codewords per block; groups of blocks × data codewords

var qrVersionsM = [11]qrBlocks{
	{},
	{10, 1, 16, 0, 0}, {16, 1, 28, 0, 0}, {26, 1, 44, 0, 0}, {18, 2, 32, 0, 0}, {24, 2, 43, 0, 0},
	{16, 4, 27, 0, 0}, {18, 4, 31, 0, 0}, {22, 2, 38, 2, 39}, {22, 3, 36, 2, 37}, {26, 4, 43, 1, 44},
}

var qrAlign = [11][]int{nil, nil, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34}, {6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50}}

// ---- GF(256) Reed–Solomon

var gfExp [512]byte
var gfLog [256]byte

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func rsECC(data []byte, n int) []byte {
	gen := []byte{1}
	for i := 0; i < n; i++ {
		next := make([]byte, len(gen)+1)
		for j, g := range gen {
			next[j] ^= g
			next[j+1] ^= gfMul(g, gfExp[i])
		}
		gen = next
	}
	rem := make([]byte, n)
	for _, d := range data {
		f := d ^ rem[0]
		copy(rem, rem[1:])
		rem[n-1] = 0
		for j := 0; j < n; j++ {
			rem[j] ^= gfMul(gen[j+1], f)
		}
	}
	return rem
}

// ---- matrix

type qrMatrix struct {
	n   int
	mod [][]bool
	fn  [][]bool // function patterns (not data)
}

func newQRMatrix(ver int) *qrMatrix {
	n := 17 + 4*ver
	m := &qrMatrix{n: n, mod: make([][]bool, n), fn: make([][]bool, n)}
	for i := range m.mod {
		m.mod[i] = make([]bool, n)
		m.fn[i] = make([]bool, n)
	}
	return m
}

func (m *qrMatrix) set(x, y int, dark bool) { m.mod[y][x] = dark; m.fn[y][x] = true }

func (m *qrMatrix) finder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= m.n || y >= m.n {
				continue
			}
			d := max(abs(dx), abs(dy))
			m.set(x, y, d != 2 && d != 4)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m *qrMatrix) functionPatterns(ver int) {
	m.finder(3, 3)
	m.finder(m.n-4, 3)
	m.finder(3, m.n-4)
	for i := 8; i < m.n-8; i++ {
		m.set(i, 6, i%2 == 0)
		m.set(6, i, i%2 == 0)
	}
	al := qrAlign[ver]
	for i, ax := range al {
		for j, ay := range al {
			if (i == 0 && j == 0) || (i == 0 && j == len(al)-1) || (i == len(al)-1 && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					m.set(ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	// reserve format areas (filled later) and the dark module
	m.format(0)
	m.set(8, m.n-8, true)
	if ver >= 7 {
		rem := ver
		for i := 0; i < 12; i++ {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := ver<<12 | rem
		for i := 0; i < 18; i++ {
			dark := (bits>>i)&1 != 0
			a, b := m.n-11+i%3, i/3
			m.set(a, b, dark)
			m.set(b, a, dark)
		}
	}
}

// format writes the 15-bit format info for level M and the given mask.
func (m *qrMatrix) format(mask int) {
	data := 0<<3 | mask // level M = 00
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>i)&1 != 0 }
	for i := 0; i <= 5; i++ {
		m.set(8, i, bit(i))
	}
	m.set(8, 7, bit(6))
	m.set(8, 8, bit(7))
	m.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		m.set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		m.set(m.n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		m.set(8, m.n-15+i, bit(i))
	}
}

func (m *qrMatrix) placeData(cw []byte) {
	i := 0
	total := len(cw) * 8
	for right := m.n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < m.n; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				upward := ((right + 1) & 2) == 0
				y := vert
				if upward {
					y = m.n - 1 - vert
				}
				if m.fn[y][x] {
					continue
				}
				if i < total {
					m.mod[y][x] = (cw[i>>3]>>(7-i&7))&1 != 0
					i++
				}
			}
		}
	}
}

func qrMaskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

func (m *qrMatrix) applyMask(mask int) {
	for y := 0; y < m.n; y++ {
		for x := 0; x < m.n; x++ {
			if !m.fn[y][x] && qrMaskBit(mask, x, y) {
				m.mod[y][x] = !m.mod[y][x]
			}
		}
	}
}

func (m *qrMatrix) penalty() int {
	n, p := m.n, 0
	get := func(x, y int, col bool) bool {
		if col {
			return m.mod[x][y]
		}
		return m.mod[y][x]
	}
	for _, col := range []bool{false, true} {
		for y := 0; y < n; y++ {
			run := 1
			for x := 1; x < n; x++ {
				if get(x, y, col) == get(x-1, y, col) {
					run++
					if run == 5 {
						p += 3
					} else if run > 5 {
						p++
					}
				} else {
					run = 1
				}
			}
			for x := 0; x+6 < n; x++ {
				pat := true
				for k, v := range []bool{true, false, true, true, true, false, true} {
					if get(x+k, y, col) != v {
						pat = false
						break
					}
				}
				if !pat {
					continue
				}
				lightBefore, lightAfter := true, true
				for k := 1; k <= 4; k++ {
					if x-k >= 0 && get(x-k, y, col) {
						lightBefore = false
					}
					if x+6+k < n && get(x+6+k, y, col) {
						lightAfter = false
					}
				}
				if lightBefore || lightAfter {
					p += 40
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if m.mod[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				c := m.mod[y][x]
				if m.mod[y][x+1] == c && m.mod[y+1][x] == c && m.mod[y+1][x+1] == c {
					p += 3
				}
			}
		}
	}
	pct := dark * 100 / (n * n)
	p += abs(pct-50) / 5 * 10
	return p
}

// QREncode returns the module matrix (true = dark) for data.
func QREncode(data []byte) ([][]bool, error) {
	ver := 0
	for v := 1; v <= 10; v++ {
		b := qrVersionsM[v]
		capBytes := b.g1*b.d1 + b.g2*b.d2
		head := 4 + 8
		if v >= 10 {
			head = 4 + 16
		}
		if (head+8*len(data)+7)/8 <= capBytes {
			ver = v
			break
		}
	}
	if ver == 0 {
		return nil, errors.New("qr: data too long")
	}
	b := qrVersionsM[ver]
	capBytes := b.g1*b.d1 + b.g2*b.d2

	// bit stream
	var bits []bool
	put := func(v, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (v>>i)&1 != 0)
		}
	}
	put(0b0100, 4)
	if ver >= 10 {
		put(len(data), 16)
	} else {
		put(len(data), 8)
	}
	for _, c := range data {
		put(int(c), 8)
	}
	put(0, min(4, capBytes*8-len(bits)))
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	cw := make([]byte, 0, capBytes)
	for i := 0; i < len(bits); i += 8 {
		var c byte
		for j := 0; j < 8; j++ {
			if bits[i+j] {
				c |= 1 << (7 - j)
			}
		}
		cw = append(cw, c)
	}
	for pad := 0; len(cw) < capBytes; pad++ {
		if pad%2 == 0 {
			cw = append(cw, 0xEC)
		} else {
			cw = append(cw, 0x11)
		}
	}

	// blocks + interleave
	var blocks, eccs [][]byte
	off := 0
	for g := 0; g < 2; g++ {
		cnt, size := b.g1, b.d1
		if g == 1 {
			cnt, size = b.g2, b.d2
		}
		for i := 0; i < cnt; i++ {
			blk := cw[off : off+size]
			off += size
			blocks = append(blocks, blk)
			eccs = append(eccs, rsECC(blk, b.ec))
		}
	}
	var final []byte
	maxD := max(b.d1, b.d2)
	for i := 0; i < maxD; i++ {
		for _, blk := range blocks {
			if i < len(blk) {
				final = append(final, blk[i])
			}
		}
	}
	for i := 0; i < b.ec; i++ {
		for _, e := range eccs {
			final = append(final, e[i])
		}
	}

	best, bestP := -1, 0
	var out *qrMatrix
	for mask := 0; mask < 8; mask++ {
		m := newQRMatrix(ver)
		m.functionPatterns(ver)
		m.placeData(final)
		m.applyMask(mask)
		m.format(mask)
		if p := m.penalty(); best < 0 || p < bestP {
			best, bestP, out = mask, p, m
		}
	}
	return out.mod, nil
}

// QRPNG renders data as a PNG with a 4-module quiet zone.
func QRPNG(data string, scale int) ([]byte, error) {
	mod, err := QREncode([]byte(data))
	if err != nil {
		return nil, err
	}
	n := len(mod)
	size := (n + 8) * scale
	img := image.NewGray(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if !mod[y][x] {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetGray((x+4)*scale+dx, (y+4)*scale+dy, color.Gray{Y: 0})
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
