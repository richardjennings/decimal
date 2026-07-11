package decimal

import (
	"bufio"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func encodeContext(width int) Context {
	f := dpdFormatFor(width)
	var emax, emin int32
	switch width {
	case 4:
		emax, emin = 96, -95
	case 8:
		emax, emin = 384, -383
	case 16:
		emax, emin = 6144, -6143
	}
	return Context{
		Precision:   int32(f.prec),
		Rounding:    RoundHalfEven,
		MaxExponent: emax,
		MinExponent: emin,
		Clamp:       true,
		Bounded:     true,
	}
}

func TestEncodeVectors(t *testing.T) {
	hard := []struct {
		s, hexStr string
		width     int
	}{
		{"-7.50", "A23003D0", 4},
		{"-7.50E+3", "A26003D0", 4},
		{"Infinity", "78000000", 4},
		{"NaN", "7C000000", 4},
		{"-7.50", "A2300000000003D0", 8},
		{"-7.50E+3", "A23C0000000003D0", 8},
		{"0", "2238000000000000", 8},
		{"0E-398", "0000000000000000", 8},
		{"1E-398", "0000000000000001", 8},
		{"1.0E-397", "0000000000000010", 8},
		{"1E-397", "0004000000000001", 8},
		{"9999999999999999E-398", "6400FF3FCFF3FCFF", 8},
		{"-1E-398", "8000000000000001", 8},
		{"-7.50", "A20780000000000000000000000003D0", 16},
		{"-7.50E+3", "A20840000000000000000000000003D0", 16},
		{"-750", "A20800000000000000000000000003D0", 16},
	}
	for _, v := range hard {
		raw, _ := hex.DecodeString(v.hexStr)
		// Compare by value: some vectors give a non-canonical spelling (e.g.
		// 9999999999999999E-398, whose canonical to-scientific form differs).
		if got := Decode(raw); !Identical(got, MustParse(v.s)) {
			t.Errorf("hardcoded decode %s: got %q want %q", v.hexStr, got, v.s)
		}
		d, err := NewFromString(v.s)
		if err != nil {
			t.Fatalf("NewFromString(%q): %v", v.s, err)
		}
		rd, _ := encodeContext(v.width).RoundToContext(d)
		got := strings.ToUpper(hex.EncodeToString(rd.Encode(v.width)))
		if got != strings.ToUpper(v.hexStr) {
			t.Errorf("hardcoded encode %q: got %s want %s", v.s, got, v.hexStr)
		}
	}

	suite := filepath.Join("dectest", "testdata", "suite")
	if _, err := os.Stat(suite); err != nil {
		t.Logf("suite directory %s not present; skipping file vectors", suite)
		return
	}

	files := []string{
		"dsEncode.decTest", "ddEncode.decTest", "dqEncode.decTest",
		"dsBase.decTest", "ddBase.decTest", "dqBase.decTest",
	}
	var pass, fail, skip int
	var mismatches []string
	for _, name := range files {
		p, f, s, m := runEncodeFile(t, filepath.Join(suite, name))
		pass, fail, skip = pass+p, fail+f, skip+s
		mismatches = append(mismatches, m...)
	}
	t.Logf("file vectors: pass=%d fail=%d skip=%d", pass, fail, skip)
	for i, m := range mismatches {
		if i >= 20 {
			t.Logf("... and %d more mismatches", len(mismatches)-20)
			break
		}
		t.Errorf("%s", m)
	}
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

func runEncodeFile(t *testing.T, path string) (pass, fail, skip int, mismatches []string) {
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	sc := bufio.NewScanner(file)
	base := filepath.Base(path)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[3] != "->" {
			continue
		}
		if !strings.EqualFold(fields[1], "apply") {
			continue
		}
		id := fields[0]
		operand := unquote(fields[2])
		result := unquote(fields[4])
		opHex := strings.HasPrefix(operand, "#")
		resHex := strings.HasPrefix(result, "#")

		switch {
		case opHex && resHex:
			// Canonicalisation: decode a (possibly non-canonical) encoding and
			// re-encode. Decode->Encode preserves signaling NaNs, unlike
			// RoundToContext, which quiets them.
			raw, err := hex.DecodeString(strings.ToLower(operand[1:]))
			if err != nil {
				skip++
				continue
			}
			got := hex.EncodeToString(Decode(raw).Encode(len(operand[1:]) / 2))
			if strings.EqualFold(got, result[1:]) {
				pass++
			} else {
				fail++
				mismatches = append(mismatches, base+" "+id+": canon "+operand+" got #"+got+" want "+result)
			}
		case opHex:
			raw, err := hex.DecodeString(strings.ToLower(operand[1:]))
			if err != nil {
				skip++
				continue
			}
			got := Decode(raw).String()
			if got == result {
				pass++
			} else {
				fail++
				mismatches = append(mismatches, base+" "+id+": decode "+operand+" got "+got+" want "+result)
			}
		case resHex:
			width := len(result[1:]) / 2
			d, err := NewFromString(operand)
			if err != nil {
				skip++
				continue
			}
			rd, _ := encodeContext(width).RoundToContext(d)
			got := hex.EncodeToString(rd.Encode(width))
			if strings.EqualFold(got, result[1:]) {
				pass++
			} else {
				fail++
				mismatches = append(mismatches, base+" "+id+": encode "+operand+" got #"+got+" want "+result)
			}
		default:
			skip++
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return
}
