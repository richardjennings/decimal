package decimal

import (
	"math/big"
	"strings"
)

func infinity(neg bool) Decimal { return Decimal{form: infinite, neg: neg} }

func makeNaN(neg bool, payload *big.Int, signaling bool) Decimal {
	d := Decimal{neg: neg, form: quietNaN}
	if signaling {
		d.form = signalingNaN
	}
	if payload != nil {
		d.coeff.Set(payload)
	}
	return d
}

func (d Decimal) isInf() bool { return d.form == infinite }
func (d Decimal) isNaN() bool { return d.form == quietNaN || d.form == signalingNaN }

// IsNaN reports whether d is a NaN (quiet or signaling).
func (d Decimal) IsNaN() bool  { return d.isNaN() }
func (d Decimal) isZero() bool { return d.form == finite && d.coeff.Sign() == 0 }

// sign reports whether d is negative. Finite non-zero values carry their sign
// in the coefficient; zero and non-finite forms carry it in neg (signed zero).
func (d Decimal) sign() bool {
	if d.form == finite && d.coeff.Sign() != 0 {
		return d.coeff.Sign() < 0
	}
	return d.neg
}

// zeroAddSign gives the sign of a zero sum: the common sign if the operands
// agree, otherwise negative only under round-floor (GDA addition rule).
func zeroAddSign(a, b Decimal, mode RoundingMode) bool {
	if a.sign() == b.sign() {
		return a.sign()
	}
	return mode == RoundFloor
}

// infRank orders infinities: -1 for -Infinity, +1 for +Infinity, 0 for finite.
func infRank(d Decimal) int {
	if d.isInf() {
		if d.neg {
			return -1
		}
		return 1
	}
	return 0
}

// cmpDecimal orders two non-NaN values, with -Infinity < finite < +Infinity.
func cmpDecimal(a, b Decimal) int {
	ra, rb := infRank(a), infRank(b)
	switch {
	case ra != rb:
		if ra < rb {
			return -1
		}
		return 1
	case ra != 0:
		return 0
	}
	// Bounded alignment: keeping max-digits+2 places means equal truncated parts
	// can only arise from an exact (untruncated) alignment, so the sticky tail
	// never changes a non-zero comparison and can be ignored.
	keep := int64(numDigits(&a.coeff))
	if n := int64(numDigits(&b.coeff)); n > keep {
		keep = n
	}
	ca, cb, _, _ := alignBounded(a, b, keep+2)
	return ca.Cmp(cb)
}

// nanResult applies GDA NaN propagation: a signaling NaN raises
// Invalid_operation and yields a quiet NaN; otherwise a quiet NaN propagates.
// The diagnostic payload is truncated to precision-1 digits.
func nanResult(prec int32, ops ...Decimal) (Decimal, Condition, bool) {
	for _, d := range ops {
		if d.form == signalingNaN {
			return makeNaN(d.neg, truncPayload(&d.coeff, prec), false), InvalidOperation, true
		}
	}
	for _, d := range ops {
		if d.form == quietNaN {
			return makeNaN(d.neg, truncPayload(&d.coeff, prec), false), 0, true
		}
	}
	return Decimal{}, 0, false
}

// truncPayload keeps the low-order precision-1 digits of a NaN payload.
func truncPayload(p *big.Int, prec int32) *big.Int {
	if prec <= 0 || numDigits(p) <= int(prec) {
		return p
	}
	return new(big.Int).Mod(p, pow10(uint(prec)))
}

func parseSpecial(s string) (Decimal, bool) {
	neg, t := false, s
	switch {
	case strings.HasPrefix(t, "-"):
		neg, t = true, t[1:]
	case strings.HasPrefix(t, "+"):
		t = t[1:]
	}
	switch low := strings.ToLower(t); {
	case low == "inf" || low == "infinity":
		return infinity(neg), true
	case strings.HasPrefix(low, "snan"):
		if p, ok := parsePayload(t[4:]); ok {
			return makeNaN(neg, p, true), true
		}
	case strings.HasPrefix(low, "nan"):
		if p, ok := parsePayload(t[3:]); ok {
			return makeNaN(neg, p, false), true
		}
	}
	return Decimal{}, false
}

// parsePayload parses a NaN diagnostic payload, which must be all digits.
func parsePayload(s string) (*big.Int, bool) {
	if s == "" {
		return nil, true
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return nil, false
		}
	}
	p, _ := new(big.Int).SetString(s, 10)
	return p, true
}
