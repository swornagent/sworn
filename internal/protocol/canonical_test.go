package protocol

import (
	"io/fs"
	"math"
	"testing"
)

func TestCanonicalizeJSONMatchesBatonVectors(t *testing.T) {
	t.Parallel()

	input := []byte("{\"€\":\"Euro\",\"\\r\":\"Carriage Return\",\"דּ\":\"Hebrew\",\"1\":\"One\",\"😀\":\"Emoji\",\"\":\"Control\",\"ö\":\"Latin\"}")
	want := "{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\":\"Control\",\"ö\":\"Latin\",\"€\":\"Euro\",\"😀\":\"Emoji\",\"דּ\":\"Hebrew\"}"
	canonical, err := CanonicalizeJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != want {
		t.Fatalf("canonical JSON = %q, want %q", canonical, want)
	}

	values := []float64{333333333.33333329, 1e30, 4.50, 0.002, 1e-27}
	wantNumbers := []string{"333333333.3333333", "1e+30", "4.5", "0.002", "1e-27"}
	for index, value := range values {
		actual, err := formatJCSNumber(value)
		if err != nil || actual != wantNumbers[index] {
			t.Errorf("formatJCSNumber(%g) = %q, %v; want %q", value, actual, err, wantNumbers[index])
		}
	}
}

func TestJCSNumberFormattingMatchesRFC8785AppendixB(t *testing.T) {
	t.Parallel()

	for bits, want := range map[uint64]string{
		0x0000000000000000: "0",
		0x8000000000000000: "0",
		0x0000000000000001: "5e-324",
		0x8000000000000001: "-5e-324",
		0x7fefffffffffffff: "1.7976931348623157e+308",
		0xffefffffffffffff: "-1.7976931348623157e+308",
		0x4340000000000000: "9007199254740992",
		0xc340000000000000: "-9007199254740992",
		0x4430000000000000: "295147905179352830000",
		0x44b52d02c7e14af5: "9.999999999999997e+22",
		0x44b52d02c7e14af6: "1e+23",
		0x44b52d02c7e14af7: "1.0000000000000001e+23",
		0x444b1ae4d6e2ef4e: "999999999999999700000",
		0x444b1ae4d6e2ef4f: "999999999999999900000",
		0x444b1ae4d6e2ef50: "1e+21",
		0x3eb0c6f7a0b5ed8c: "9.999999999999997e-7",
		0x3eb0c6f7a0b5ed8d: "0.000001",
		0x41b3de4355555553: "333333333.3333332",
		0x41b3de4355555554: "333333333.33333325",
		0x41b3de4355555555: "333333333.3333333",
		0x41b3de4355555556: "333333333.3333334",
		0x41b3de4355555557: "333333333.33333343",
		0xbecbf647612f3696: "-0.0000033333333333333333",
		0x43143ff3c1cb0959: "1424953923781206.2",
	} {
		value := math.Float64frombits(bits)
		got, err := formatJCSNumber(value)
		if err != nil || got != want {
			t.Errorf("formatJCSNumber(%016x) = %q, %v; want %q", bits, got, err, want)
		}
	}
}

func TestCanonicalizeJSONRejectsNonIJSON(t *testing.T) {
	t.Parallel()

	for name, input := range map[string][]byte{
		"duplicate":      []byte(`{"value":1,"value":2}`),
		"unsafe integer": []byte(`{"value":9007199254740992}`),
		"unsafe float":   []byte(`{"value":1e20}`),
		"lone surrogate": []byte(`{"value":"\ud800"}`),
		"invalid UTF-8":  {'{', '"', 0xff, '"', ':', '1', '}'},
		"second value":   []byte(`{} {}`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := CanonicalizeJSON(input); err == nil {
				t.Fatal("invalid I-JSON was accepted")
			}
		})
	}
	if _, err := formatJCSNumber(math.Inf(1)); err == nil {
		t.Fatal("infinite number was accepted")
	}
	if _, err := EncodeCanonical(map[string]string{"value": string([]byte{0xff})}); err == nil {
		t.Fatal("invalid UTF-8 Go string was replaced during canonical encoding")
	}
	backing := []string{"valid", string([]byte{0xff})}
	if _, err := EncodeCanonical([][]string{backing[:1], backing[:2]}); err == nil {
		t.Fatal("invalid UTF-8 in a longer aliased slice was skipped")
	}
}

func TestStandardSubmissionCanonicalDigestMatchesSnapshot(t *testing.T) {
	t.Parallel()

	snapshot, err := SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/standard-submission.json")
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		t.Fatal(err)
	}
	const want = "sha256:51532765e47ad1d3414a7753e025ac17646cbbae70cd8ec63f5f3487b125a2f6"
	if digest := CanonicalDigest(canonical); digest != want {
		t.Fatalf("submission digest = %q, want %q", digest, want)
	}
}
