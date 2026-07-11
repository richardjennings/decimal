package decimal

import "math/big"

// shiftToExp expresses coeff (at exponent from) at the exponent to. When to > from
// it shifts right, returning the truncated coefficient and a sticky flag that is
// true when non-zero digits were dropped. It never materialises pow10 of more
// than the coefficient's own length.
func shiftToExp(coeff *big.Int, from, to int64) (*big.Int, bool) {
	if coeff.Sign() == 0 {
		return big.NewInt(0), false // zero shifts to zero at any exponent
	}
	switch {
	case from >= to:
		return new(big.Int).Mul(coeff, pow10(uint(from-to))), false
	case to-from >= int64(numDigits(coeff)):
		// the whole coefficient lies below `to`: it becomes zero plus a sticky bit
		return big.NewInt(0), coeff.Sign() != 0
	default:
		q := new(big.Int)
		r := new(big.Int)
		q.QuoRem(coeff, pow10(uint(to-from)), r)
		return q, r.Sign() != 0
	}
}

// alignBounded scales a and b to a common exponent, but never below
// (max adjusted exponent - keep), so the operands' magnitudes are bounded even
// when their exponents span billions. Digits dropped below the window set sticky.
func alignBounded(a, b Decimal, keep int64) (ca, cb *big.Int, e int32, sticky bool) {
	lo := int64(a.exp)
	if int64(b.exp) < lo {
		lo = int64(b.exp)
	}
	// Only operands with significant digits anchor the top of the window: a zero
	// contributes nothing, so its exponent must not force the other to truncate.
	haveAdj, adjMax := false, int64(0)
	if a.coeff.Sign() != 0 {
		adjMax, haveAdj = int64(a.exp)+int64(numDigits(&a.coeff))-1, true
	}
	if b.coeff.Sign() != 0 {
		if adjB := int64(b.exp) + int64(numDigits(&b.coeff)) - 1; !haveAdj || adjB > adjMax {
			adjMax, haveAdj = adjB, true
		}
	}
	if haveAdj {
		if cut := adjMax - keep; lo < cut {
			lo = cut
		}
	}
	e = clampExp(lo)
	ca, sa := shiftToExp(&a.coeff, int64(a.exp), int64(e))
	cb, sb := shiftToExp(&b.coeff, int64(b.exp), int64(e))
	return ca, cb, e, sa || sb
}

// smallerNeg reports the sign of the operand with the smaller adjusted exponent
// (the one that gets truncated by alignBounded).
func smallerNeg(a, b Decimal) bool {
	adjA := int64(a.exp) + int64(numDigits(&a.coeff)) - 1
	adjB := int64(b.exp) + int64(numDigits(&b.coeff)) - 1
	if adjA < adjB {
		return a.sign()
	}
	return b.sign()
}

// roundToOddTail folds a truncated non-zero tail (with sign tailNeg and magnitude
// below one unit in the last place of sum) back into sum, so that a single later
// rounding is correct for every rounding mode. It first truncates sum toward zero
// — when the tail opposes sum the true value is one ULP smaller in magnitude —
// then, if the last digit is even, steps it toward the true value to make it odd.
func roundToOddTail(sum *big.Int, tailNeg bool) *big.Int {
	one := big.NewInt(1)
	sign := sum.Sign()
	vNeg := tailNeg
	if sign != 0 {
		vNeg = sign < 0
		if (sign < 0) != tailNeg { // tail opposes sum: |true value| is |sum|-1
			if sign > 0 {
				sum.Sub(sum, one)
			} else {
				sum.Add(sum, one)
			}
		}
	}
	if sum.Bit(0) == 0 {
		if vNeg {
			sum.Sub(sum, one)
		} else {
			sum.Add(sum, one)
		}
	}
	return sum
}
