package qr

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// The strongest thing a test can say about an encoder without a decoder to
// lean on is that its output decodes: the format and version fields carry
// valid BCH codes, every block's Reed–Solomon syndromes are zero, and the
// data read back out of the zigzag is the data that went in. The syndromes
// are computed by evaluating each block at the generator's roots, which is a
// different calculation from the polynomial division the encoder does.
func TestEncodedSymbolsDecode(t *testing.T) {
	inputs := []string{
		"",
		"A",
		"otpauth://totp/Obsidian%20Arc:arc?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=Obsidian%20Arc&algorithm=SHA1&digits=6&period=30",
		// A Chinese issuer percent-encodes to three times its length, which
		// is what pushes a real link past version 9 and into the 16-bit
		// length field.
		"otpauth://totp/" + strings.Repeat("%E9%BB%91%E6%9B%9C", 12) + ":arc?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
		strings.Repeat("0123456789abcdef", 60),
	}
	for _, input := range inputs {
		code, err := Encode([]byte(input))
		if err != nil {
			t.Fatalf("encode %d bytes: %v", len(input), err)
		}
		got := decode(t, code)
		if !bytes.Equal(got, []byte(input)) {
			t.Fatalf("round trip of %d bytes came back as %q", len(input), got)
		}
	}
}

// Level M holds 14 bytes in version 1 and 2331 in version 40; one more than
// either is the next version, or nothing at all.
func TestVersionIsTheSmallestThatFits(t *testing.T) {
	for _, tc := range []struct {
		length, size int
	}{
		{14, 21},
		{15, 25},
		{2331, 177},
	} {
		code, err := Encode(bytes.Repeat([]byte("x"), tc.length))
		if err != nil {
			t.Fatalf("%d bytes: %v", tc.length, err)
		}
		if code.Size != tc.size {
			t.Errorf("%d bytes drew a %d-module symbol, want %d", tc.length, code.Size, tc.size)
		}
	}
	if _, err := Encode(bytes.Repeat([]byte("x"), 2332)); err != ErrTooLong {
		t.Errorf("2332 bytes: got %v, want ErrTooLong", err)
	}
}

// A scanner finds the symbol by its three finders before it reads anything.
func TestFindersSitInThreeCorners(t *testing.T) {
	code, err := Encode([]byte("finder"))
	if err != nil {
		t.Fatal(err)
	}
	for _, corner := range [][2]int{{0, 0}, {code.Size - 7, 0}, {0, code.Size - 7}} {
		for dy := 0; dy < 7; dy++ {
			for dx := 0; dx < 7; dx++ {
				ring := max(abs(dx-3), abs(dy-3))
				want := ring != 2
				if code.Dark(corner[0]+dx, corner[1]+dy) != want {
					t.Fatalf("finder at %v is wrong at (%d,%d)", corner, dx, dy)
				}
			}
		}
	}
}

func TestPathCoversEveryDarkModuleOnce(t *testing.T) {
	code, err := Encode([]byte("path"))
	if err != nil {
		t.Fatal(err)
	}
	dark := 0
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Dark(x, y) {
				dark++
			}
		}
	}
	// Each run is "M x y h n v1 h-n z", so the widths add up to the count.
	covered := 0
	for _, segment := range strings.Split(code.Path(), "M")[1:] {
		var x, y, n int
		if _, err := fmt.Sscanf(segment, "%d %dh%d", &x, &y, &n); err != nil {
			t.Fatalf("unreadable segment %q: %v", segment, err)
		}
		covered += n
	}
	if covered != dark {
		t.Errorf("path covers %d modules, symbol has %d dark", covered, dark)
	}
}

// --- a decoder, just enough to check the encoder -----------------------------

func decode(t *testing.T, code Code) []byte {
	t.Helper()
	size := code.Size
	version := (size - 17) / 4

	// Format information, first copy, read in the order it is written.
	formatAt := [15][2]int{}
	for i := 0; i <= 5; i++ {
		formatAt[i] = [2]int{8, i}
	}
	formatAt[6] = [2]int{8, 7}
	formatAt[7] = [2]int{8, 8}
	formatAt[8] = [2]int{7, 8}
	for i := 9; i < 15; i++ {
		formatAt[i] = [2]int{14 - i, 8}
	}
	format := 0
	for i, at := range formatAt {
		if code.Dark(at[0], at[1]) {
			format |= 1 << i
		}
	}
	// The second copy must agree with the first.
	second := 0
	for i := 0; i < 8; i++ {
		if code.Dark(size-1-i, 8) {
			second |= 1 << i
		}
	}
	for i := 8; i < 15; i++ {
		if code.Dark(8, size-15+i) {
			second |= 1 << i
		}
	}
	if second != format {
		t.Fatalf("the two copies of the format disagree: %015b vs %015b", format, second)
	}
	format ^= 0x5412
	if bchRemainder(format>>10, 0x537, 10) != format&0x3FF {
		t.Fatalf("format bits %015b are not a BCH codeword", format)
	}
	if level := format >> 13; level != 0 {
		t.Fatalf("level bits are %02b, want M (00)", level)
	}
	mask := (format >> 10) & 7

	if version >= 7 {
		bits := 0
		for i := 0; i < 18; i++ {
			if code.Dark(size-11+i%3, i/3) {
				bits |= 1 << i
			}
		}
		if bits>>12 != version {
			t.Fatalf("version field says %d, size says %d", bits>>12, version)
		}
		if bchRemainder(version, 0x1F25, 12) != bits&0xFFF {
			t.Fatalf("version bits %018b are not a BCH codeword", bits)
		}
	}

	// Which modules are data is geometry, and the encoder's own map is the
	// statement of it; the checks above are what make that not circular.
	layout := newSymbol(version)
	layout.drawFunctionPatterns()
	copy(layout.modules, code.modules)
	layout.applyMask(mask)

	var stream []byte
	var current byte
	count := 0
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		upward := (right+1)&2 == 0
		for vert := 0; vert < size; vert++ {
			y := vert
			if upward {
				y = size - 1 - vert
			}
			for j := 0; j < 2; j++ {
				x := right - j
				if layout.function[y*size+x] {
					continue
				}
				current <<= 1
				if layout.modules[y*size+x] {
					current |= 1
				}
				count++
				if count%8 == 0 {
					stream = append(stream, current)
					current = 0
				}
			}
		}
	}
	raw := rawModules(version) / 8
	if len(stream) < raw {
		t.Fatalf("read %d codewords, want %d", len(stream), raw)
	}
	stream = stream[:raw]

	blocks := eccBlocks[version]
	eccLen := eccPerBlock[version]
	short := blocks - raw%blocks
	shortLen := raw / blocks
	split := make([][]byte, blocks)
	k := 0
	for i := 0; i < shortLen+1; i++ {
		for j := 0; j < blocks; j++ {
			if i == shortLen-eccLen && j < short {
				continue
			}
			split[j] = append(split[j], stream[k])
			k++
		}
	}
	var data []byte
	for j, block := range split {
		for root := 0; root < eccLen; root++ {
			if syndrome(block, root) != 0 {
				t.Fatalf("block %d has a non-zero syndrome at α^%d", j, root)
			}
		}
		data = append(data, block[:len(block)-eccLen]...)
	}

	reader := bitReader{data: data}
	if mode := reader.read(4); mode != 0b0100 {
		t.Fatalf("mode indicator %04b, want byte mode", mode)
	}
	length := reader.read(countBits(version))
	out := make([]byte, length)
	for i := range out {
		out[i] = byte(reader.read(8))
	}
	return out
}

func bchRemainder(data, generator, degree int) int {
	rem := data
	for i := 0; i < degree; i++ {
		rem = (rem << 1) ^ ((rem >> (degree - 1)) * generator)
	}
	return rem & (1<<degree - 1)
}

// syndrome evaluates the block, highest coefficient first, at α^root.
func syndrome(block []byte, root int) byte {
	point := byte(1)
	for i := 0; i < root; i++ {
		point = gfMultiply(point, 2)
	}
	var sum byte
	for _, b := range block {
		sum = gfMultiply(sum, point) ^ b
	}
	return sum
}

type bitReader struct {
	data []byte
	pos  int
}

func (r *bitReader) read(count int) int {
	value := 0
	for i := 0; i < count; i++ {
		value <<= 1
		if r.data[r.pos/8]>>(7-r.pos%8)&1 == 1 {
			value |= 1
		}
		r.pos++
	}
	return value
}
