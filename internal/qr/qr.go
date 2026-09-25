// Package qr draws a QR code: byte mode, error correction level M, the
// smallest version that holds the data, and the mask the standard's penalty
// rules prefer.
//
// It exists for one picture — the otpauth:// link an authenticator app scans
// when two-step sign-in is switched on — and is written here rather than
// imported because a dependency for a few hundred lines of arithmetic is the
// trade AGENTS.md says not to make. It is drawn on the server rather than in
// the browser so the encoder is not on the first paint of everyone who only
// came to chat.
//
// Nothing here is clever. The layout follows ISO/IEC 18004 in the order the
// standard describes it, with the version tables reduced to the two rows level
// M needs; everything else in them is arithmetic on the version number.
package qr

import (
	"errors"
	"strconv"
	"strings"
)

// ErrTooLong is returned for data a version-40 symbol cannot hold. At level M
// that is 2331 bytes, which an otpauth link never approaches.
var ErrTooLong = errors.New("qr: data too long for a QR code")

// Code is a square of modules, true for dark. Size excludes the quiet zone;
// whoever draws it adds the four-module margin the standard asks for.
type Code struct {
	Size    int
	modules []bool
}

// Dark reports one module. Outside the symbol is light, which is what the
// quiet zone around it is.
func (c Code) Dark(x, y int) bool {
	if x < 0 || y < 0 || x >= c.Size || y >= c.Size {
		return false
	}
	return c.modules[y*c.Size+x]
}

// Path is the dark modules as one SVG path in module units, a rectangle per
// horizontal run. One element rather than one per module, because a version
// that fits a long issuer name has several thousand of them.
func (c Code) Path() string {
	var out strings.Builder
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; {
			if !c.Dark(x, y) {
				x++
				continue
			}
			run := 1
			for c.Dark(x+run, y) {
				run++
			}
			out.WriteString("M" + strconv.Itoa(x) + " " + strconv.Itoa(y) +
				"h" + strconv.Itoa(run) + "v1h-" + strconv.Itoa(run) + "z")
			x += run
		}
	}
	return out.String()
}

// Level M's rows of the standard's table, indexed by version: error
// correction codewords per block, and how many blocks. Index 0 is unused.
var (
	eccPerBlock = [41]int{0,
		10, 16, 26, 18, 24, 16, 18, 22, 22, 26,
		30, 22, 22, 24, 24, 28, 28, 26, 26, 26,
		26, 28, 28, 28, 28, 28, 28, 28, 28, 28,
		28, 28, 28, 28, 28, 28, 28, 28, 28, 28}
	eccBlocks = [41]int{0,
		1, 1, 1, 2, 2, 4, 4, 4, 5, 5,
		5, 8, 9, 9, 10, 10, 11, 13, 14, 16,
		17, 17, 18, 20, 21, 23, 25, 26, 28, 29,
		31, 33, 35, 37, 38, 40, 43, 45, 47, 49}
)

// Level M's two format bits. The standard's order is L, M, Q, H → 01, 00, 11, 10.
const levelBits = 0

// Encode picks the smallest version that holds data and draws it.
func Encode(data []byte) (Code, error) {
	version := 0
	for v := 1; v <= 40; v++ {
		if 4+countBits(v)+8*len(data) <= dataCodewords(v)*8 {
			version = v
			break
		}
	}
	if version == 0 {
		return Code{}, ErrTooLong
	}

	symbol := newSymbol(version)
	symbol.drawFunctionPatterns()
	symbol.drawCodewords(interleave(version, dataStream(version, data)))

	// Every mask gives a readable symbol. The penalty only prefers the one
	// with the fewest shapes a scanner could mistake for something else.
	best, bestScore := 0, -1
	for mask := 0; mask < 8; mask++ {
		symbol.applyMask(mask)
		symbol.drawFormatBits(mask)
		if score := symbol.penalty(); bestScore < 0 || score < bestScore {
			best, bestScore = mask, score
		}
		// XOR again, which takes the mask back off.
		symbol.applyMask(mask)
	}
	symbol.applyMask(best)
	symbol.drawFormatBits(best)

	return Code{Size: symbol.size, modules: symbol.modules}, nil
}

// countBits is the width of the byte-mode length field.
func countBits(version int) int {
	if version <= 9 {
		return 8
	}
	return 16
}

// rawModules is how many modules are left for data and error correction once
// every function pattern is drawn. Closed-form, per the standard's geometry.
func rawModules(version int) int {
	result := (16*version+128)*version + 64
	if version >= 2 {
		align := version/7 + 2
		result -= (25*align-10)*align - 55
		if version >= 7 {
			result -= 36
		}
	}
	return result
}

func dataCodewords(version int) int {
	return rawModules(version)/8 - eccPerBlock[version]*eccBlocks[version]
}

// dataStream is the mode indicator, the length, the bytes, then the
// terminator and the alternating pad bytes the standard fills with.
func dataStream(version int, data []byte) []byte {
	var bits bitWriter
	bits.write(0b0100, 4)
	bits.write(len(data), countBits(version))
	for _, b := range data {
		bits.write(int(b), 8)
	}
	capacity := dataCodewords(version) * 8
	bits.write(0, min(4, capacity-bits.length))
	bits.write(0, (8-bits.length%8)%8)
	for pad := 0xEC; bits.length < capacity; pad ^= 0xEC ^ 0x11 {
		bits.write(pad, 8)
	}
	return bits.bytes
}

type bitWriter struct {
	bytes  []byte
	length int
}

func (w *bitWriter) write(value, count int) {
	for i := count - 1; i >= 0; i-- {
		if w.length%8 == 0 {
			w.bytes = append(w.bytes, 0)
		}
		if (value>>i)&1 == 1 {
			w.bytes[w.length/8] |= 0x80 >> (w.length % 8)
		}
		w.length++
	}
}

// interleave splits the data into the version's blocks, appends each block's
// Reed–Solomon codewords, and reads the blocks out column by column. Short
// blocks come first and are one data codeword shorter than the long ones.
func interleave(version int, data []byte) []byte {
	blocks := eccBlocks[version]
	eccLen := eccPerBlock[version]
	raw := rawModules(version) / 8
	short := blocks - raw%blocks
	shortLen := raw / blocks

	divisor := rsDivisor(eccLen)
	split := make([][]byte, blocks)
	for i, k := 0, 0; i < blocks; i++ {
		n := shortLen - eccLen
		if i >= short {
			n++
		}
		chunk := data[k : k+n]
		k += n
		// Every block is laid out at the long length, so a short block has a
		// gap before its error correction that the read-out below skips.
		block := make([]byte, shortLen+1)
		copy(block, chunk)
		copy(block[len(block)-eccLen:], rsRemainder(chunk, divisor))
		split[i] = block
	}

	out := make([]byte, 0, raw)
	for i := 0; i < shortLen+1; i++ {
		for j := 0; j < blocks; j++ {
			if i == shortLen-eccLen && j < short {
				continue
			}
			out = append(out, split[j][i])
		}
	}
	return out
}

// rsDivisor is the generator polynomial of the given degree, highest term
// dropped, coefficients from the x^(degree-1) term down.
func rsDivisor(degree int) []byte {
	result := make([]byte, degree)
	result[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := range result {
			result[j] = gfMultiply(result[j], root)
			if j+1 < len(result) {
				result[j] ^= result[j+1]
			}
		}
		root = gfMultiply(root, 0x02)
	}
	return result
}

func rsRemainder(data, divisor []byte) []byte {
	result := make([]byte, len(divisor))
	for _, b := range data {
		factor := b ^ result[0]
		copy(result, result[1:])
		result[len(result)-1] = 0
		for i := range result {
			result[i] ^= gfMultiply(divisor[i], factor)
		}
	}
	return result
}

// gfMultiply is multiplication in GF(2^8) modulo x^8 + x^4 + x^3 + x^2 + 1,
// the field the standard's error correction is defined over.
func gfMultiply(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= int((y>>i)&1) * int(x)
	}
	return byte(z)
}

// symbol is the square being drawn, with a second layer that records which
// modules belong to a function pattern so data and masks leave them alone.
type symbol struct {
	version  int
	size     int
	modules  []bool
	function []bool
}

func newSymbol(version int) *symbol {
	size := version*4 + 17
	return &symbol{
		version:  version,
		size:     size,
		modules:  make([]bool, size*size),
		function: make([]bool, size*size),
	}
}

func (s *symbol) set(x, y int, dark bool) {
	s.modules[y*s.size+x] = dark
	s.function[y*s.size+x] = true
}

func (s *symbol) drawFunctionPatterns() {
	for i := 0; i < s.size; i++ {
		s.set(6, i, i%2 == 0)
		s.set(i, 6, i%2 == 0)
	}

	s.drawFinder(3, 3)
	s.drawFinder(s.size-4, 3)
	s.drawFinder(3, s.size-4)

	positions := s.alignmentPositions()
	last := len(positions) - 1
	for i, x := range positions {
		for j, y := range positions {
			// Three of the grid's corners are where the finders already are.
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue
			}
			s.drawAlignment(x, y)
		}
	}

	// Reserves the format areas (and draws the one module that is always
	// dark) so the data placement steps around them; the real bits are
	// written once a mask has been chosen.
	s.drawFormatBits(0)
	s.drawVersion()
}

// drawFinder draws a finder pattern and its light separator, clipped to the
// symbol.
func (s *symbol) drawFinder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			x, y := cx+dx, cy+dy
			if x < 0 || y < 0 || x >= s.size || y >= s.size {
				continue
			}
			ring := max(abs(dx), abs(dy))
			s.set(x, y, ring != 2 && ring != 4)
		}
	}
}

func (s *symbol) drawAlignment(cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			s.set(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1)
		}
	}
}

// alignmentPositions are the row and column centres of the alignment
// patterns: 6, then evenly spaced by an even step back from the far edge.
func (s *symbol) alignmentPositions() []int {
	if s.version == 1 {
		return nil
	}
	count := s.version/7 + 2
	step := (s.version*8 + count*3 + 5) / (count*4 - 4) * 2
	out := make([]int, count)
	out[0] = 6
	for i, pos := count-1, s.size-7; i >= 1; i, pos = i-1, pos-step {
		out[i] = pos
	}
	return out
}

// drawFormatBits writes the level and mask, protected by a BCH(15,5) code,
// in both of the places the standard keeps a copy.
func (s *symbol) drawFormatBits(mask int) {
	data := levelBits<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412

	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	for i := 0; i <= 5; i++ {
		s.set(8, i, bit(i))
	}
	s.set(8, 7, bit(6))
	s.set(8, 8, bit(7))
	s.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		s.set(14-i, 8, bit(i))
	}

	for i := 0; i < 8; i++ {
		s.set(s.size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		s.set(8, s.size-15+i, bit(i))
	}
	s.set(8, s.size-8, true)
}

// drawVersion writes the version, protected by a BCH(18,6) code, beside the
// two far finders. Versions below 7 have no version area.
func (s *symbol) drawVersion() {
	if s.version < 7 {
		return
	}
	rem := s.version
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	bits := s.version<<12 | rem
	for i := 0; i < 18; i++ {
		dark := (bits>>i)&1 == 1
		a, b := s.size-11+i%3, i/3
		s.set(a, b, dark)
		s.set(b, a, dark)
	}
}

// drawCodewords lays the bits out in the standard's zigzag: two-module
// columns from the right edge, alternately upward and downward, stepping over
// the vertical timing pattern and every function module.
func (s *symbol) drawCodewords(data []byte) {
	i := 0
	for right := s.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		upward := (right+1)&2 == 0
		for vert := 0; vert < s.size; vert++ {
			y := vert
			if upward {
				y = s.size - 1 - vert
			}
			for j := 0; j < 2; j++ {
				x := right - j
				if s.function[y*s.size+x] || i >= len(data)*8 {
					continue
				}
				s.modules[y*s.size+x] = (data[i/8]>>(7-i%8))&1 == 1
				i++
			}
		}
	}
}

// applyMask flips every data module the mask's condition selects. Applying
// the same mask twice undoes it, which is how the candidates are tried.
func (s *symbol) applyMask(mask int) {
	for y := 0; y < s.size; y++ {
		for x := 0; x < s.size; x++ {
			if s.function[y*s.size+x] {
				continue
			}
			var flip bool
			switch mask {
			case 0:
				flip = (x+y)%2 == 0
			case 1:
				flip = y%2 == 0
			case 2:
				flip = x%3 == 0
			case 3:
				flip = (x+y)%3 == 0
			case 4:
				flip = (x/3+y/2)%2 == 0
			case 5:
				flip = x*y%2+x*y%3 == 0
			case 6:
				flip = (x*y%2+x*y%3)%2 == 0
			case 7:
				flip = ((x+y)%2+x*y%3)%2 == 0
			}
			if flip {
				s.modules[y*s.size+x] = !s.modules[y*s.size+x]
			}
		}
	}
}

// penalty scores the symbol by the standard's four rules: long runs of one
// colour, 2×2 blocks of one colour, shapes that look like a finder, and an
// imbalance between dark and light.
func (s *symbol) penalty() int {
	dark := func(x, y int) bool { return s.modules[y*s.size+x] }
	score := 0

	for pass := 0; pass < 2; pass++ {
		at := dark
		if pass == 1 {
			at = func(x, y int) bool { return dark(y, x) }
		}
		for y := 0; y < s.size; y++ {
			run := 1
			for x := 1; x <= s.size; x++ {
				if x < s.size && at(x, y) == at(x-1, y) {
					run++
					continue
				}
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
			}
			for x := 0; x+10 < s.size; x++ {
				if finderLike(func(i int) bool { return at(x+i, y) }) {
					score += 40
				}
			}
		}
	}

	for y := 0; y+1 < s.size; y++ {
		for x := 0; x+1 < s.size; x++ {
			c := dark(x, y)
			if c == dark(x+1, y) && c == dark(x, y+1) && c == dark(x+1, y+1) {
				score += 3
			}
		}
	}

	count := 0
	for _, m := range s.modules {
		if m {
			count++
		}
	}
	total := s.size * s.size
	k := (abs(count*20-total*10)+total-1)/total - 1
	score += k * 10
	return score
}

// finderLike matches the eleven modules 1:1:3:1:1 with four light modules on
// either side, the proportion a scanner hunts for.
func finderLike(at func(int) bool) bool {
	pattern := [7]bool{true, false, true, true, true, false, true}
	light := func(from int) bool {
		for i := from; i < from+4; i++ {
			if at(i) {
				return false
			}
		}
		return true
	}
	matches := func(from int) bool {
		for i, want := range pattern {
			if at(from+i) != want {
				return false
			}
		}
		return true
	}
	return (matches(0) && light(7)) || (light(0) && matches(4))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
