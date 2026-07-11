package decimal

// nextStep returns the value adjacent to finite a in the direction dir
// (RoundCeiling toward +Infinity, RoundFloor toward -Infinity), with the
// conditions from the underlying rounding. It adds a quantum just below a's ULP
// and lets directed rounding land on the neighbouring value; the quantum's
// exponent stays within precision of a, so alignment never blows up.
func (c Context) nextStep(a Decimal, dir RoundingMode) (Decimal, Condition) {
	work := c
	work.Rounding = dir
	rounded, cond := work.RoundToContext(a)
	if cond&Inexact != 0 {
		// Directed rounding changed the value: that IS the neighbour (and its
		// exponent is now bounded for the alignment below).
		return rounded, cond
	}
	// a is exactly representable (trailing zeros may have been stripped): step by
	// one ULP in the direction.
	etiny := int64(c.MinExponent) - int64(c.Precision) + 1
	if rounded.coeff.Sign() == 0 { // next from zero is the smallest subnormal
		res := New(1, clampExp(etiny))
		if dir == RoundFloor {
			res = res.Neg()
		}
		return res, Subnormal | Underflow | Inexact | Rounded
	}
	ulpExp := int64(rounded.exp) + int64(numDigits(&rounded.coeff)) - 1 - int64(c.Precision) + 1
	if ulpExp < etiny {
		ulpExp = etiny
	}
	tiny := New(1, clampExp(ulpExp-1))
	if dir == RoundFloor {
		tiny = tiny.Neg()
	}
	return work.Add(rounded, tiny)
}

// NextPlus returns the next value toward +Infinity (quiet except for sNaN).
func (c Context) NextPlus(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isInf() {
		if a.neg {
			return c.maxFinite(true), 0
		}
		return infinity(false), 0
	}
	res, _ := c.nextStep(a, RoundCeiling)
	return res, 0
}

// NextMinus returns the next value toward -Infinity (quiet except for sNaN).
func (c Context) NextMinus(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isInf() {
		if a.neg {
			return infinity(true), 0
		}
		return c.maxFinite(false), 0
	}
	res, _ := c.nextStep(a, RoundFloor)
	return res, 0
}

// NextToward returns the next value from a toward b, signalling like an
// arithmetic operation.
func (c Context) NextToward(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	cmp := cmpDecimal(a, b)
	if cmp == 0 {
		res, _ := c.RoundToContext(a.CopySign(b)) // equal operands: quiet, sign of b
		return res, 0
	}
	if a.isInf() {
		return c.maxFinite(a.neg), 0
	}
	dir := RoundCeiling
	if cmp > 0 {
		dir = RoundFloor
	}
	res, _ := c.nextStep(a, dir)
	// The signals depend on the class of the result, not the input.
	var cond Condition
	switch {
	case res.isInf():
		cond = Overflow | Inexact | Rounded
	case res.coeff.Sign() == 0: // stepped across zero (underflow)
		cond = Subnormal | Underflow | Inexact | Rounded | Clamped
	case c.subnormalFlag(res) != 0:
		cond = Subnormal | Underflow | Inexact | Rounded
	}
	return res, cond
}
