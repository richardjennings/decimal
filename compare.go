package decimal

import "math/big"

// --- copy operations: raw value manipulation, no rounding, no signals ---

// Copy returns a unchanged.
func (a Decimal) Copy() Decimal { return a }

// CopyAbs returns |a| (sign cleared), without rounding or signalling.
func (a Decimal) CopyAbs() Decimal {
	if a.sign() {
		return a.Neg()
	}
	return a
}

// CopySign returns a with the sign of b.
func (a Decimal) CopySign(b Decimal) Decimal {
	if a.sign() != b.sign() {
		return a.Neg()
	}
	return a
}

// --- total ordering (IEEE 754 totalOrder) ---

// rank orders the forms for the total order: finite < Infinity < sNaN < NaN.
func rank(d Decimal) int {
	switch d.form {
	case finite:
		return 0
	case infinite:
		return 1
	case signalingNaN:
		return 2
	default:
		return 3
	}
}

// compareTotalMag compares |a| and |b| in total order, returning -1, 0, or 1.
func compareTotalMag(a, b Decimal) int {
	if ra, rb := rank(a), rank(b); ra != rb {
		if ra < rb {
			return -1
		}
		return 1
	}
	switch a.form {
	case finite:
		if cv := cmpDecimal(a.CopyAbs(), b.CopyAbs()); cv != 0 {
			return cv
		}
		switch { // equal value: larger exponent is greater
		case a.exp > b.exp:
			return 1
		case a.exp < b.exp:
			return -1
		}
		return 0
	case infinite:
		return 0
	default: // NaN: order by payload
		return new(big.Int).Abs(&a.coeff).Cmp(new(big.Int).Abs(&b.coeff))
	}
}

func compareTotalOrder(a, b Decimal) int {
	if a.sign() != b.sign() {
		if a.sign() {
			return -1
		}
		return 1
	}
	r := compareTotalMag(a, b)
	if a.sign() {
		return -r
	}
	return r
}

// CompareTotal orders a and b by the total order, returning -1, 0, or 1.
func (c Context) CompareTotal(a, b Decimal) (Decimal, Condition) {
	return New(int64(compareTotalOrder(a, b)), 0), 0
}

// CompareTotMag orders |a| and |b| by the total order.
func (c Context) CompareTotMag(a, b Decimal) (Decimal, Condition) {
	return New(int64(compareTotalMag(a, b)), 0), 0
}

// CompareSig is Compare, but any NaN operand (quiet or signaling) raises Invalid.
func (c Context) CompareSig(a, b Decimal) (Decimal, Condition) {
	if a.isNaN() || b.isNaN() {
		r, cond, _ := nanResult(c.Precision, a, b)
		return r, cond | InvalidOperation
	}
	return New(int64(cmpDecimal(a, b)), 0), 0
}

// --- max / min family ---

// selectNaN applies max/min NaN rules: a quiet NaN is ignored when the other
// operand is a number; sNaN or two NaNs propagate.
func (c Context) selectNaN(a, b Decimal) (Decimal, Condition, bool) {
	if a.form == signalingNaN || b.form == signalingNaN || (a.isNaN() && b.isNaN()) {
		r, cond, _ := nanResult(c.Precision, a, b)
		return r, cond, true
	}
	if a.isNaN() {
		d, k := c.RoundToContext(b)
		return d, k, true
	}
	if b.isNaN() {
		d, k := c.RoundToContext(a)
		return d, k, true
	}
	return Decimal{}, 0, false
}

func (c Context) Max(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := c.selectNaN(a, b); ok {
		return r, cond
	}
	cmp := cmpDecimal(a, b)
	if cmp == 0 {
		cmp = compareTotalOrder(a, b)
	}
	if cmp >= 0 {
		return c.RoundToContext(a)
	}
	return c.RoundToContext(b)
}

func (c Context) Min(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := c.selectNaN(a, b); ok {
		return r, cond
	}
	cmp := cmpDecimal(a, b)
	if cmp == 0 {
		cmp = compareTotalOrder(a, b)
	}
	if cmp <= 0 {
		return c.RoundToContext(a)
	}
	return c.RoundToContext(b)
}

func (c Context) MaxMag(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := c.selectNaN(a, b); ok {
		return r, cond
	}
	cmp := cmpDecimal(a.CopyAbs(), b.CopyAbs())
	if cmp == 0 {
		cmp = compareTotalOrder(a, b)
	}
	if cmp >= 0 {
		return c.RoundToContext(a)
	}
	return c.RoundToContext(b)
}

func (c Context) MinMag(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := c.selectNaN(a, b); ok {
		return r, cond
	}
	cmp := cmpDecimal(a.CopyAbs(), b.CopyAbs())
	if cmp == 0 {
		cmp = compareTotalOrder(a, b)
	}
	if cmp <= 0 {
		return c.RoundToContext(a)
	}
	return c.RoundToContext(b)
}

// SameQuantum reports (as 1 or 0) whether a and b have the same exponent.
func (c Context) SameQuantum(a, b Decimal) (Decimal, Condition) {
	var same bool
	switch {
	case a.isNaN() && b.isNaN():
		same = true
	case a.isInf() && b.isInf():
		same = true
	case a.form == finite && b.form == finite:
		same = a.exp == b.exp
	}
	if same {
		return New(1, 0), 0
	}
	return New(0, 0), 0
}
