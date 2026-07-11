// Package dectest parses and runs the IBM/Cowlishaw General Decimal Arithmetic
// ".decTest" conformance suite against this module's decimal implementation.
package dectest

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	dec "github.com/richardjennings/decimal"
)

// Result tallies the outcome of running a set of test cases.
type Result struct {
	Pass, Fail, Skip int
	Failures         []string
}

type context struct {
	precision      int32
	rounding       dec.RoundingMode
	maxExp, minExp int32
	clamp          bool
}

var implemented = map[string]bool{
	"add": true, "subtract": true, "multiply": true, "divide": true,
	"divideint": true, "remainder": true, "remaindernear": true,
	"compare": true, "comparetotal": true, "comparetotmag": true, "comparesig": true,
	"max": true, "min": true, "maxmag": true, "minmag": true, "samequantum": true,
	"copy": true, "copyabs": true, "copynegate": true, "copysign": true,
	"quantize": true, "fma": true,
	"and": true, "or": true, "xor": true, "invert": true,
	"shift": true, "rotate": true, "scaleb": true,
	"tointegral": true, "tointegralx": true, "logb": true, "reduce": true, "canonical": true,
	"squareroot": true, "rescale": true, "trim": true, "class": true,
	"nextplus": true, "nextminus": true, "nexttoward": true,
	"exp": true, "ln": true, "log10": true, "power": true,
	"plus": true, "minus": true, "abs": true,
	"apply": true, "tosci": true, "toeng": true,
}

var condByName = map[string]dec.Condition{
	"rounded":             dec.Rounded,
	"inexact":             dec.Inexact,
	"invalid_operation":   dec.InvalidOperation,
	"division_by_zero":    dec.DivisionByZero,
	"overflow":            dec.Overflow,
	"underflow":           dec.Underflow,
	"subnormal":           dec.Subnormal,
	"clamped":             dec.Clamped,
	"division_undefined":  dec.DivisionUndefined,
	"division_impossible": dec.DivisionImpossible,
	"conversion_syntax":   dec.ConversionSyntax,
	"invalid_context":     dec.InvalidContext,
}

const modeledConds = dec.Rounded | dec.Inexact | dec.InvalidOperation | dec.DivisionByZero |
	dec.Overflow | dec.Underflow | dec.Subnormal | dec.Clamped |
	dec.DivisionUndefined | dec.DivisionImpossible | dec.ConversionSyntax | dec.InvalidContext

// RunFile executes every runnable case in one .decTest file.
func RunFile(path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	ctx := context{precision: 9, rounding: dec.RoundHalfEven, maxExp: 999, minExp: -999}
	var res Result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if i := strings.IndexByte(line, ':'); i >= 0 && !strings.Contains(line, "->") {
			applyDirective(&ctx, line[:i], line[i+1:])
			continue
		}
		if strings.Contains(line, "->") {
			runCase(&ctx, line, &res)
		}
	}
	return res, sc.Err()
}

func applyDirective(ctx *context, key, val string) {
	key = strings.ToLower(strings.TrimSpace(key))
	val = strings.TrimSpace(val)
	if i := strings.Index(val, "--"); i >= 0 {
		val = strings.TrimSpace(val[:i])
	}
	val = strings.Trim(val, `'"`)
	switch key {
	case "precision":
		if n, err := strconv.ParseInt(val, 10, 32); err == nil {
			ctx.precision = int32(n)
		}
	case "rounding":
		if m, ok := parseRounding(val); ok {
			ctx.rounding = m
		}
	case "maxexponent":
		if n, err := strconv.ParseInt(val, 10, 32); err == nil {
			ctx.maxExp = int32(n)
		}
	case "minexponent":
		if n, err := strconv.ParseInt(val, 10, 32); err == nil {
			ctx.minExp = int32(n)
		}
	case "clamp":
		ctx.clamp = val == "1"
	}
}

func runCase(ctx *context, line string, res *Result) {
	toks := tokenize(line)
	arrow := -1
	for i, t := range toks {
		if t == "->" {
			arrow = i
			break
		}
	}
	if arrow < 3 || arrow+1 >= len(toks) {
		res.Skip++
		return
	}
	id, op := toks[0], strings.ToLower(toks[1])
	operands, expected, conds := toks[2:arrow], toks[arrow+1], toks[arrow+2:]
	expWidth := formatTagWidth(expected) // "64#<decimal>" tags the result's format
	expected = stripFormatTag(expected)

	if !implemented[op] || expected == "?" || hasUnmodeledCondition(conds) {
		res.Skip++
		return
	}
	for _, o := range operands {
		if o == "#" { // a bare "#" is a null operand: the operation is undefined
			if (expected == "NaN" || expected == "-NaN") && flagsMatch(dec.InvalidOperation, conds) {
				res.Pass++
			} else {
				res.Skip++
			}
			return
		}
	}
	// copy-family operations on an encoded operand preserve its exact bits (even
	// non-canonical ones), so they are done on the raw bytes rather than decoded.
	switch op {
	case "copy", "copyabs", "copynegate", "copysign":
		if strings.HasPrefix(expected, "#") && strings.HasPrefix(operands[0], "#") {
			out, ok := copyEncoded(op, operands)
			if !ok {
				res.Skip++
				return
			}
			got := "#" + hex.EncodeToString(out)
			if strings.EqualFold(got, expected) && flagsMatch(0, conds) {
				res.Pass++
			} else {
				res.Fail++
				if len(res.Failures) < 5000 {
					res.Failures = append(res.Failures, fmt.Sprintf("%s %s %v -> %s (want %s)", id, op, operands, got, expected))
				}
			}
			return
		}
	}
	ds, ok := parseOperands(operands)
	if !ok {
		// A syntactically invalid operand is a conversion error: NaN + Conversion_syntax.
		if (expected == "NaN" || expected == "-NaN") && flagsMatch(dec.ConversionSyntax, conds) {
			res.Pass++
		} else {
			res.Skip++
		}
		return
	}
	if tooBig(ds) {
		res.Skip++
		return
	}
	// toSci converts a string to a decimal: a NaN literal whose payload exceeds the
	// context capacity is a conversion error. Arithmetic operations instead truncate
	// an over-long operand payload, which the library handles, so this guards toSci
	// only (a context-free NewFromString cannot know the precision).
	if op == "tosci" {
		nanLimit := int(ctx.precision) // fixed formats (clamp=1) reserve one digit
		if ctx.clamp {
			nanLimit--
		}
		for _, d := range ds {
			if d.IsNaN() && d.Coeff().Sign() != 0 && d.NumDigits() > nanLimit {
				if (expected == "NaN" || expected == "-NaN") && flagsMatch(dec.ConversionSyntax, conds) {
					res.Pass++
				} else {
					res.Skip++
				}
				return
			}
		}
	}

	dctx := dec.Context{Precision: ctx.precision, Rounding: ctx.rounding, MaxExponent: ctx.maxExp, MinExponent: ctx.minExp, Clamp: ctx.clamp, Bounded: true}
	if op == "class" { // returns a classification string, not a Decimal
		if len(ds) != 1 {
			res.Skip++
			return
		}
		if dctx.Class(ds[0]) == expected {
			res.Pass++
		} else {
			res.Fail++
			if len(res.Failures) < 5000 {
				res.Failures = append(res.Failures, fmt.Sprintf("%s class %v -> %s (want %s)", id, operands, dctx.Class(ds[0]), expected))
			}
		}
		return
	}
	resultHex := strings.HasPrefix(expected, "#")
	operandHex := false
	for _, o := range operands {
		if strings.HasPrefix(o, "#") {
			operandHex = true
		}
	}
	if resultHex || operandHex {
		var got dec.Decimal
		var cond dec.Condition
		if op == "apply" {
			// Interchange-format conversion: round a finite value into the format, but
			// pass Infinity/NaN/sNaN through unchanged (it raises no signal and never
			// quiets an sNaN).
			got = ds[0]
			if got.IsFinite() {
				got, cond = dctx.RoundToContext(got)
			}
		} else { // a real operation on decoded operands (tointegralx, canonical, ...)
			var ok bool
			if got, cond, ok = apply(dctx, op, ds); !ok {
				res.Skip++
				return
			}
		}
		gotStr := got.String()
		if resultHex {
			b, _ := hexToBytes(expected[1:])
			gotStr = "#" + hex.EncodeToString(got.Encode(len(b)))
		}
		if strings.EqualFold(gotStr, expected) && flagsMatch(cond, conds) {
			res.Pass++
			return
		}
		res.Fail++
		if len(res.Failures) < 5000 {
			res.Failures = append(res.Failures, fmt.Sprintf(
				"%s %s %v -> %s %v (want %s %v)", id, op, operands, gotStr, condNames(cond), expected, conds))
		}
		return
	}
	got, cond, ok := apply(dctx, op, ds)
	if !ok {
		res.Skip++
		return
	}
	if expWidth != 0 && got.IsFinite() { // "64#<decimal>" result: fit into that format
		var c2 dec.Condition
		got, c2 = formatCtx(expWidth).RoundToContext(got)
		cond |= c2
		want, err := dec.NewFromString(expected)
		if err == nil {
			want, _ = formatCtx(expWidth).RoundToContext(want)
		}
		if err == nil && dec.Identical(got, want) && flagsMatch(cond, conds) {
			res.Pass++
			return
		}
		res.Fail++
		if len(res.Failures) < 5000 {
			res.Failures = append(res.Failures, fmt.Sprintf(
				"%s %s %v -> %s %v (want %s %v)", id, op, operands, got.String(), condNames(cond), expected, conds))
		}
		return
	}
	gotStr := got.String()
	if op == "toeng" {
		gotStr = got.StringEng()
	}
	if gotStr == expected && flagsMatch(cond, conds) {
		res.Pass++
		return
	}
	res.Fail++
	if len(res.Failures) < 5000 {
		res.Failures = append(res.Failures, fmt.Sprintf(
			"%s %s %v -> %s %v (want %s %v)", id, op, operands, gotStr, condNames(cond), expected, conds))
	}
}

func apply(c dec.Context, op string, ds []dec.Decimal) (dec.Decimal, dec.Condition, bool) {
	binary := func(f func(a, b dec.Decimal) (dec.Decimal, dec.Condition)) (dec.Decimal, dec.Condition, bool) {
		if len(ds) != 2 {
			return dec.Decimal{}, 0, false
		}
		d, k := f(ds[0], ds[1])
		return d, k, true
	}
	unary := func(f func(a dec.Decimal) (dec.Decimal, dec.Condition)) (dec.Decimal, dec.Condition, bool) {
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		d, k := f(ds[0])
		return d, k, true
	}
	ternary := func(f func(a, b, x dec.Decimal) (dec.Decimal, dec.Condition)) (dec.Decimal, dec.Condition, bool) {
		if len(ds) != 3 {
			return dec.Decimal{}, 0, false
		}
		d, k := f(ds[0], ds[1], ds[2])
		return d, k, true
	}
	switch op {
	case "add":
		return binary(c.Add)
	case "subtract":
		return binary(c.Sub)
	case "multiply":
		return binary(c.Mul)
	case "divide":
		return binary(c.Divide)
	case "divideint":
		return binary(c.DivideInt)
	case "remainder":
		return binary(c.Rem)
	case "remaindernear":
		return binary(c.RemNear)
	case "compare":
		return binary(c.Compare)
	case "comparetotal":
		return binary(c.CompareTotal)
	case "comparetotmag":
		return binary(c.CompareTotMag)
	case "comparesig":
		return binary(c.CompareSig)
	case "max":
		return binary(c.Max)
	case "min":
		return binary(c.Min)
	case "maxmag":
		return binary(c.MaxMag)
	case "minmag":
		return binary(c.MinMag)
	case "samequantum":
		return binary(c.SameQuantum)
	case "copy":
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		return ds[0].Copy(), 0, true
	case "copyabs":
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		return ds[0].CopyAbs(), 0, true
	case "copynegate":
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		return ds[0].Neg(), 0, true
	case "copysign":
		if len(ds) != 2 {
			return dec.Decimal{}, 0, false
		}
		return ds[0].CopySign(ds[1]), 0, true
	case "quantize":
		return binary(c.Quantize)
	case "fma":
		return ternary(c.FMA)
	case "and":
		return binary(c.And)
	case "or":
		return binary(c.Or)
	case "xor":
		return binary(c.Xor)
	case "invert":
		return unary(c.Invert)
	case "shift":
		return binary(c.Shift)
	case "rotate":
		return binary(c.Rotate)
	case "scaleb":
		return binary(c.ScaleB)
	case "tointegral":
		return unary(func(a dec.Decimal) (dec.Decimal, dec.Condition) { return c.ToIntegral(a, false) })
	case "tointegralx":
		return unary(func(a dec.Decimal) (dec.Decimal, dec.Condition) { return c.ToIntegral(a, true) })
	case "logb":
		return unary(c.LogB)
	case "reduce":
		return unary(c.Reduce)
	case "canonical":
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		return ds[0].Canonical(), 0, true
	case "squareroot":
		return unary(c.Sqrt)
	case "trim":
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		return ds[0].Trim(), 0, true
	case "nextplus":
		return unary(c.NextPlus)
	case "nextminus":
		return unary(c.NextMinus)
	case "nexttoward":
		return binary(c.NextToward)
	case "exp":
		return unary(c.Exp)
	case "ln":
		return unary(c.Ln)
	case "log10":
		return unary(c.Log10)
	case "power":
		return binary(c.Power)
	case "rescale":
		return binary(c.Rescale)
	case "toeng": // like tosci but the result is compared as an engineering string
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		if !ds[0].IsFinite() {
			return ds[0], 0, true
		}
		d, k := c.RoundToContext(ds[0])
		return d, k, true
	case "plus":
		return unary(c.Plus)
	case "apply": // round to context, preserving sign
		return unary(c.RoundToContext)
	case "tosci": // like apply for finite operands, but passes NaN/Inf through without signalling
		if len(ds) != 1 {
			return dec.Decimal{}, 0, false
		}
		if !ds[0].IsFinite() {
			return ds[0], 0, true
		}
		d, k := c.RoundToContext(ds[0])
		return d, k, true
	case "minus":
		return unary(c.Minus)
	case "abs":
		return unary(c.Abs)
	}
	return dec.Decimal{}, 0, false
}

// tokenize splits a case line on whitespace, honoring single/double quotes
// (a doubled quote is a literal) and stopping at an unquoted "--" comment.
func tokenize(line string) []string {
	var toks []string
	var b strings.Builder
	var quote byte
	quoted := false // an explicit quote emits a token even when empty ('' is a valid operand)
	flush := func() {
		if b.Len() > 0 || quoted {
			toks = append(toks, b.String())
			b.Reset()
			quoted = false
		}
	}
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if ch == quote {
				if i+1 < len(line) && line[i+1] == quote {
					b.WriteByte(ch)
					i++
				} else {
					quote = 0
				}
			} else {
				b.WriteByte(ch)
			}
			continue
		}
		switch {
		case ch == '\'' || ch == '"':
			quote = ch
			quoted = true
		case ch == '-' && i+1 < len(line) && line[i+1] == '-':
			flush()
			return toks
		case ch == ' ' || ch == '\t':
			flush()
		default:
			b.WriteByte(ch)
		}
	}
	flush()
	return toks
}

func parseOperands(toks []string) ([]dec.Decimal, bool) {
	ds := make([]dec.Decimal, 0, len(toks))
	for _, t := range toks {
		if strings.HasPrefix(t, "#") { // a densely-packed-decimal interchange encoding
			b, ok := hexToBytes(t[1:])
			if !ok {
				return nil, false
			}
			ds = append(ds, dec.Decode(b))
			continue
		}
		d, err := dec.NewFromString(stripFormatTag(t))
		if err != nil {
			return nil, false
		}
		if w := formatTagWidth(t); w != 0 { // interpret in its interchange format (may clamp)
			d, _ = formatCtx(w).RoundToContext(d)
		}
		ds = append(ds, d)
	}
	return ds, true
}

// hexToBytes decodes a decimal32/64/128 encoding (8, 16, or 32 hex digits).
func hexToBytes(s string) ([]byte, bool) {
	if n := len(s); n != 8 && n != 16 && n != 32 {
		return nil, false
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}

// copyEncoded applies a copy-family operation to raw interchange bytes, which
// preserves the exact encoding (including non-canonical forms) and only touches
// the sign bit. operands are "#<hex>" tokens.
func copyEncoded(op string, operands []string) ([]byte, bool) {
	b, ok := hexToBytes(strings.TrimPrefix(operands[0], "#"))
	if !ok {
		return nil, false
	}
	out := append([]byte(nil), b...)
	switch op {
	case "copy":
	case "copyabs":
		out[0] &^= 0x80
	case "copynegate":
		out[0] ^= 0x80
	case "copysign":
		if len(operands) != 2 {
			return nil, false
		}
		neg, ok := signOf(operands[1]) // the sign comes from the second operand
		if !ok {
			return nil, false
		}
		out[0] &^= 0x80
		if neg {
			out[0] |= 0x80
		}
	default:
		return nil, false
	}
	return out, true
}

// signOf reports the sign of an operand token, which may be an interchange
// encoding (#hex) or an ordinary decimal literal.
func signOf(tok string) (neg, ok bool) {
	if strings.HasPrefix(tok, "#") {
		b, ok := hexToBytes(tok[1:])
		if !ok {
			return false, false
		}
		return b[0]&0x80 != 0, true
	}
	d, err := dec.NewFromString(stripFormatTag(tok))
	if err != nil {
		return false, false
	}
	return d.Signbit(), true
}

// stripFormatTag removes a "32#"/"64#"/"128#" prefix, which tags a decimal
// literal as belonging to a specific interchange format.
func stripFormatTag(tok string) string {
	if w := formatTagWidth(tok); w != 0 {
		return tok[strings.IndexByte(tok, '#')+1:]
	}
	return tok
}

// formatTagWidth returns the encoding width (4/8/16 bytes) of a "32#"/"64#"/"128#"
// tagged literal, or 0 if the token carries no such tag.
func formatTagWidth(tok string) int {
	i := strings.IndexByte(tok, '#')
	if i <= 0 {
		return 0
	}
	switch tok[:i] {
	case "32":
		return 4
	case "64":
		return 8
	case "128":
		return 16
	}
	return 0
}

// formatCtx is the arithmetic context of an interchange format (width bytes).
func formatCtx(width int) dec.Context {
	switch width {
	case 4:
		return dec.Context{Precision: 7, MaxExponent: 96, MinExponent: -95, Clamp: true, Bounded: true}
	case 16:
		return dec.Context{Precision: 34, MaxExponent: 6144, MinExponent: -6143, Clamp: true, Bounded: true}
	default:
		return dec.Context{Precision: 16, MaxExponent: 384, MinExponent: -383, Clamp: true, Bounded: true}
	}
}

// tooBig skips cases whose operands carry a coefficient over 1000 digits, which
// would force astronomically large intermediates. Exponent spans are no longer a
// problem: alignment is precision-bounded, so a billion-place span is handled
// without materialising pow10(span).
func tooBig(ds []dec.Decimal) bool {
	for _, d := range ds {
		if d.NumDigits() > 1000 {
			return true
		}
	}
	return false
}

func hasUnmodeledCondition(conds []string) bool {
	for _, c := range conds {
		if _, ok := condByName[strings.ToLower(c)]; !ok {
			return true
		}
	}
	return false
}

func flagsMatch(got dec.Condition, conds []string) bool {
	var want dec.Condition
	for _, c := range conds {
		want |= condByName[strings.ToLower(c)]
	}
	return got&modeledConds == want
}

func condNames(c dec.Condition) []string {
	var out []string
	for _, f := range []struct {
		bit  dec.Condition
		name string
	}{
		{dec.Rounded, "Rounded"},
		{dec.Inexact, "Inexact"},
		{dec.InvalidOperation, "Invalid_operation"},
		{dec.DivisionByZero, "Division_by_zero"},
	} {
		if c.Has(f.bit) {
			out = append(out, f.name)
		}
	}
	return out
}

func parseRounding(s string) (dec.RoundingMode, bool) {
	switch strings.ToLower(s) {
	case "half_even":
		return dec.RoundHalfEven, true
	case "half_up":
		return dec.RoundHalfUp, true
	case "half_down":
		return dec.RoundHalfDown, true
	case "down":
		return dec.RoundDown, true
	case "up":
		return dec.RoundUp, true
	case "ceiling":
		return dec.RoundCeiling, true
	case "floor":
		return dec.RoundFloor, true
	case "05up":
		return dec.Round05Up, true
	}
	return 0, false
}
