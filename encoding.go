package decimal

import (
	"math/big"
	"strings"
)

// dpdFormat holds the parameters of an IEEE 754 decimal interchange format.
type dpdFormat struct {
	width   int // encoding length in bytes
	prec    int // coefficient digits, p = 3*declets+1
	bias    int // exponent bias, subtracted from the encoded exponent
	ecbits  int // exponent-continuation bits, w
	declets int // coefficient-continuation declets, J
}

func dpdFormatFor(width int) dpdFormat {
	switch width {
	case 4:
		return dpdFormat{4, 7, 101, 6, 2}
	case 8:
		return dpdFormat{8, 16, 398, 8, 5}
	case 16:
		return dpdFormat{16, 34, 6176, 12, 11}
	}
	panic("decimal: DPD width must be 4, 8, or 16")
}

// dpdEncode maps a 3-digit value (0..999) to its canonical 10-bit declet;
// dpdDecode maps any of the 1024 declets back to its 3-digit value.
var (
	dpdEncode [1000]uint16
	dpdDecode [1024]uint16
)

func init() {
	for n := 0; n < 1000; n++ {
		dpdEncode[n] = bcdToDpd(n)
	}
	for d := 0; d < 1024; d++ {
		dpdDecode[d] = dpdToBcd(uint16(d))
	}
}

// bcdToDpd packs three decimal digits into a 10-bit declet using the standard
// DPD compression equations (bits p..y, p most significant).
func bcdToDpd(n int) uint16 {
	d1, d2, d3 := n/100, n/10%10, n%10
	a, b, c, d := d1>>3&1, d1>>2&1, d1>>1&1, d1&1
	e, f, g, h := d2>>3&1, d2>>2&1, d2>>1&1, d2&1
	i, j, k, m := d3>>3&1, d3>>2&1, d3>>1&1, d3&1
	na, ne, ni := a^1, e^1, i^1

	p := b | (a & j) | (a & f & i)
	q := c | (a & k) | (a & g & i)
	r := d
	s := (f & (na | ni)) | (na & e & j) | (e & i)
	t := g | (na & e & k) | (a & i)
	u := h
	v := a | e | i
	w := a | (e & i) | (ne & j)
	x := e | (a & i) | (na & k)
	y := m
	return uint16(p<<9 | q<<8 | r<<7 | s<<6 | t<<5 | u<<4 | v<<3 | w<<2 | x<<1 | y)
}

// dpdToBcd unpacks a 10-bit declet into a 3-digit value using the standard DPD
// expansion equations. It accepts all 1024 patterns (decode is many-to-one).
func dpdToBcd(declet uint16) uint16 {
	p := int(declet>>9) & 1
	q := int(declet>>8) & 1
	r := int(declet>>7) & 1
	s := int(declet>>6) & 1
	t := int(declet>>5) & 1
	u := int(declet>>4) & 1
	v := int(declet>>3) & 1
	w := int(declet>>2) & 1
	x := int(declet>>1) & 1
	y := int(declet) & 1
	ns, nt, nv, nw, nx := s^1, t^1, v^1, w^1, x^1

	a := (v & w) & (ns | t | nx)
	b := p & (nv | nw | (s & nt & x))
	c := q & (nv | nw | (s & nt & x))
	d := r
	e := v & ((nw & x) | (nt & x) | (s & x))
	f := (s & (nv | nx)) | (p & ns & t & v & w & x)
	g := (t & (nv | nx)) | (q & ns & t & w)
	h := u
	i := v & ((nw & nx) | (w & x & (s | t)))
	j := (nv & w) | (s & v & nw & x) | (p & w & (nx | (ns & nt)))
	k := (nv & x) | (t & nw & x) | (q & v & w & (nx | (ns & nt)))
	m := y
	d1 := a<<3 | b<<2 | c<<1 | d
	d2 := e<<3 | f<<2 | g<<1 | h
	d3 := i<<3 | j<<2 | k<<1 | m
	return uint16(d1*100 + d2*10 + d3)
}

// bitWriter packs fields MSB-first into a fixed byte slice.
type bitWriter struct {
	b   []byte
	pos int
}

func (bw *bitWriter) write(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		if v>>uint(i)&1 == 1 {
			bw.b[bw.pos>>3] |= 1 << uint(7-bw.pos&7)
		}
		bw.pos++
	}
}

// bitReader reads fields MSB-first from a byte slice.
type bitReader struct {
	b   []byte
	pos int
}

func (br *bitReader) read(n int) uint64 {
	var v uint64
	for i := 0; i < n; i++ {
		v = v<<1 | uint64(br.b[br.pos>>3]>>uint(7-br.pos&7)&1)
		br.pos++
	}
	return v
}

// magDigits returns the low n decimal digits of the non-negative magnitude,
// left-padded with zeros.
func magDigits(mag *big.Int, n int) []byte {
	s := mag.Text(10)
	out := make([]byte, n)
	for i := range out {
		out[i] = '0'
	}
	for i := 0; i < len(s) && i < n; i++ {
		out[n-1-i] = s[len(s)-1-i]
	}
	return out
}

// writeDeclets emits len(ds)/3 declets from a digit string whose length is a
// multiple of three, most-significant declet first.
func writeDeclets(bw *bitWriter, ds []byte) {
	for i := 0; i < len(ds); i += 3 {
		n := int(ds[i]-'0')*100 + int(ds[i+1]-'0')*10 + int(ds[i+2]-'0')
		bw.write(uint64(dpdEncode[n]), 10)
	}
}

// Encode returns the big-endian IEEE 754 densely-packed-decimal encoding of d.
// width is 4 (decimal32), 8 (decimal64), or 16 (decimal128). d must already be a
// finite value that fits the format's precision (7/16/34) and exponent range, or
// an Infinity/NaN. Returns width bytes.
func (d Decimal) Encode(width int) []byte {
	f := dpdFormatFor(width)
	buf := make([]byte, width)
	bw := &bitWriter{b: buf}
	sign := uint64(0)
	if d.sign() {
		sign = 1
	}
	bw.write(sign, 1)

	switch d.form {
	case infinite:
		bw.write(0x1e, 5) // combination 11110; exponent and coefficient bits zero
		return buf
	case quietNaN, signalingNaN:
		bw.write(0x1f, 5) // combination 11111
		sig := uint64(0)
		if d.form == signalingNaN {
			sig = 1
		}
		bw.write(sig<<uint(f.ecbits-1), f.ecbits) // signaling flag, then zero exponent
		writeDeclets(bw, magDigits(new(big.Int).Abs(&d.coeff), 3*f.declets))
		return buf
	}

	digits := magDigits(new(big.Int).Abs(&d.coeff), f.prec)
	msd := uint64(digits[0] - '0')
	e := int(d.exp) + f.bias
	topExp := uint64(e>>uint(f.ecbits)) & 3
	expCont := uint64(e) & (1<<uint(f.ecbits) - 1)
	var combo uint64
	if msd < 8 {
		combo = topExp<<3 | msd
	} else {
		combo = 0x18 | topExp<<1 | (msd - 8)
	}
	bw.write(combo, 5)
	bw.write(expCont, f.ecbits)
	writeDeclets(bw, digits[1:])
	return buf
}

// readCoeff reads J declets and prepends the most-significant digit, returning
// the coefficient (or NaN payload) magnitude.
func readCoeff(br *bitReader, msd, declets int) *big.Int {
	var sb strings.Builder
	sb.WriteByte(byte('0' + msd))
	for i := 0; i < declets; i++ {
		v := dpdDecode[br.read(10)]
		sb.WriteByte(byte('0' + v/100))
		sb.WriteByte(byte('0' + v/10%10))
		sb.WriteByte(byte('0' + v%10))
	}
	n, _ := new(big.Int).SetString(sb.String(), 10)
	return n
}

// Decode parses a big-endian DPD encoding (len 4/8/16) into a Decimal.
func Decode(b []byte) Decimal {
	f := dpdFormatFor(len(b))
	br := &bitReader{b: b}
	neg := br.read(1) == 1
	g := br.read(5)
	g4, g3, g2, g1, g0 := g>>4&1, g>>3&1, g>>2&1, g>>1&1, g&1

	if g4 == 1 && g3 == 1 && g2 == 1 && g1 == 1 {
		if g0 == 0 {
			return infinity(neg)
		}
		expCont := br.read(f.ecbits)
		signaling := expCont>>uint(f.ecbits-1)&1 == 1
		return makeNaN(neg, readCoeff(br, 0, f.declets), signaling)
	}

	var msd, topExp uint64
	if g4 == 1 && g3 == 1 {
		msd, topExp = 8+g0, g2<<1|g1
	} else {
		msd, topExp = g2<<2|g1<<1|g0, g4<<1|g3
	}
	e := int(topExp<<uint(f.ecbits) | br.read(f.ecbits))
	coeff := readCoeff(br, int(msd), f.declets)
	res := newDec(coeff, int32(e-f.bias))
	if neg {
		if coeff.Sign() == 0 {
			res.neg = true
		} else {
			res.coeff.Neg(&res.coeff)
		}
	}
	return res
}
