package decimal

import (
	"math/big"
	"strconv"
	"strings"
)

// String returns the canonical GDA to-scientific-string form of d.
func (d Decimal) String() string {
	switch d.form {
	case infinite:
		if d.neg {
			return "-Infinity"
		}
		return "Infinity"
	case quietNaN:
		return nanString("NaN", d)
	case signalingNaN:
		return nanString("sNaN", d)
	}
	return toSci(d.sign(), new(big.Int).Abs(&d.coeff).Text(10), d.exp)
}

func nanString(prefix string, d Decimal) string {
	if d.neg {
		prefix = "-" + prefix
	}
	if d.coeff.Sign() != 0 {
		prefix += d.coeff.Text(10)
	}
	return prefix
}

// StringEng returns the GDA to-engineering-string form (the shown exponent is a
// multiple of three, with one to three digits before the decimal point).
func (d Decimal) StringEng() string {
	if d.form != finite {
		return d.String()
	}
	return toEng(d.sign(), new(big.Int).Abs(&d.coeff).Text(10), d.exp)
}

func toEng(neg bool, digits string, exp int32) string {
	adj := int(exp) + len(digits) - 1
	if exp <= 0 && adj >= -6 {
		return toSci(neg, digits, exp) // no exponent: identical to the scientific form
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	if digits == "0" { // zero: show the exponent as a multiple of three
		r := ((int(exp) % 3) + 3) % 3
		showExp := int(exp) + (3-r)%3
		b.WriteByte('0')
		if places := showExp - int(exp); places > 0 {
			b.WriteByte('.')
			b.WriteString(strings.Repeat("0", places))
		}
		writeExp(&b, showExp)
		return b.String()
	}
	mod := ((adj % 3) + 3) % 3
	intDigits := mod + 1
	showExp := adj - mod
	if len(digits) < intDigits {
		digits += strings.Repeat("0", intDigits-len(digits))
	}
	b.WriteString(digits[:intDigits])
	if len(digits) > intDigits {
		b.WriteByte('.')
		b.WriteString(digits[intDigits:])
	}
	if showExp != 0 { // a zero engineering exponent is written plainly
		writeExp(&b, showExp)
	}
	return b.String()
}

func writeExp(b *strings.Builder, e int) {
	b.WriteByte('E')
	if e > 0 {
		b.WriteByte('+')
	}
	b.WriteString(strconv.Itoa(e))
}

// toSci implements the GDA to-scientific-string conversion for a finite value
// whose coefficient magnitude is `digits` (a decimal string) with exponent exp.
func toSci(neg bool, digits string, exp int32) string {
	adj := int(exp) + len(digits) - 1
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	switch {
	case exp <= 0 && adj >= -6:
		switch {
		case exp == 0:
			b.WriteString(digits)
		case adj >= 0:
			p := len(digits) + int(exp)
			b.WriteString(digits[:p])
			b.WriteByte('.')
			b.WriteString(digits[p:])
		default:
			b.WriteString("0.")
			b.WriteString(strings.Repeat("0", -adj-1))
			b.WriteString(digits)
		}
	default:
		b.WriteString(digits[:1])
		if len(digits) > 1 {
			b.WriteByte('.')
			b.WriteString(digits[1:])
		}
		b.WriteByte('E')
		if adj >= 0 {
			b.WriteByte('+')
		}
		b.WriteString(strconv.Itoa(adj))
	}
	return b.String()
}
