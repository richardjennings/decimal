package decimal

import "math/big"

// shift removes the drop lowest-order digits from coeff, rounding per mode.
// Returns the rounded coefficient (signed), the discarded remainder magnitude,
// and the conditions raised.
func shift(coeff *big.Int, drop int32, mode RoundingMode) (*big.Int, *big.Int, Condition) {
	if drop <= 0 {
		return new(big.Int).Set(coeff), new(big.Int), 0
	}
	divisor := pow10(uint(drop))
	neg := coeff.Sign() < 0
	mag := new(big.Int).Abs(coeff)
	q := new(big.Int)
	r := new(big.Int)
	q.QuoRem(mag, divisor, r)
	var cond Condition
	if mag.Sign() != 0 {
		cond = Rounded // digits were removed, even if they were zero
	}
	if r.Sign() != 0 {
		cond |= Inexact // a non-zero digit was removed
		if roundUp(q, r, divisor, neg, mode) {
			q.Add(q, big.NewInt(1))
		}
	}
	if neg {
		q.Neg(q)
	}
	return q, r, cond
}

// roundUp decides whether the truncated magnitude q should be incremented,
// given the discarded remainder r (< divisor), the sign, and the mode.
func roundUp(q, r, divisor *big.Int, neg bool, mode RoundingMode) bool {
	if r.Sign() == 0 {
		return false
	}
	cmp := new(big.Int).Lsh(r, 1).Cmp(divisor) // 2r vs divisor: <0 below, 0 tie, >0 above
	switch mode {
	case RoundDown:
		return false
	case RoundUp:
		return true
	case RoundCeiling:
		return !neg
	case RoundFloor:
		return neg
	case RoundHalfUp:
		return cmp >= 0
	case RoundHalfDown:
		return cmp > 0
	case RoundHalfEven:
		if cmp != 0 {
			return cmp > 0
		}
		return q.Bit(0) == 1
	case Round05Up:
		d := new(big.Int).Mod(q, big.NewInt(10)).Int64()
		return d == 0 || d == 5
	}
	return false
}

func numDigits(x *big.Int) int {
	if x.Sign() == 0 {
		return 1
	}
	s := x.Text(10)
	if s[0] == '-' {
		return len(s) - 1
	}
	return len(s)
}

func pow10(n uint) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), new(big.Int).SetUint64(uint64(n)), nil)
}
