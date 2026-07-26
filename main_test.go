package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"io"
	"log"
	"math"
	"slices"
	"testing"
)

func init() {
	log.SetOutput(io.Discard)
}

func TestInvalidKey(t *testing.T) {
	for n, tc := range []struct {
		key     string
		invalid bool
	}{
		{"commit", true},
		{"tree", true},
		{"parent", true},
		{"author", true},
		{"committer", true},
		{"encoding", true},
		{"commit ", true},
		{"non-alphanumeric", true},
		{"x", false},
		{"f00", false},
	} {
		if got, want := invalidKey(tc.key), tc.invalid; got != want {
			t.Errorf("[%d] invalidKey(%q) = %t, want %t", n, tc.key, got, want)
		}
	}
}

func TestValidPrefix(t *testing.T) {
	for n, tc := range []struct {
		prefix string
		valid  bool
	}{
		{"", false},
		{"x", false},
		{"0", true},
		{"f00", true},
		{"0000000000000000000000000000000000000000", true},   // 40 chars
		{"00000000000000000000000000000000000000000", false}, // 41 chars
	} {
		if got, want := validPrefix(tc.prefix), tc.valid; got != want {
			t.Errorf("[%d] validPrefix(%q) = %t, want %t", n, tc.prefix, got, want)
		}
	}
}

const commit = `tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100

Message
`

func TestHeadTail(t *testing.T) {
	wantHead := []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100`)

	wantTail := []byte("\n\nMessage\n")

	head, tail := headTail([]byte(commit))

	if !bytes.Equal(head, wantHead) {
		t.Errorf("head is %s, want %s", head, wantHead)
	}

	if !bytes.Equal(tail, wantTail) {
		t.Errorf("tail is %s, want %s", tail, wantTail)
	}
}

func TestFind(t *testing.T) {
	for _, tc := range []struct {
		desc          string
		startN        int
		prefix        string
		wantHash      string
		wantIteration int
		wantNewCommit []byte
		wantOK        bool
	}{
		{
			desc:          "Start at 0th iteration",
			prefix:        "0",
			startN:        0,
			wantHash:      "034c4f788c4a7522a75e1b86ee3c24eee630e822",
			wantIteration: 16,
			wantNewCommit: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
foo 16

Message
`),
			wantOK: true,
		},
		{
			desc:          "Start at final iteration",
			prefix:        "0",
			startN:        16,
			wantHash:      "034c4f788c4a7522a75e1b86ee3c24eee630e822",
			wantIteration: 16,
			wantNewCommit: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
foo 16

Message
`),
			wantOK: true,
		},
		{
			desc:          "Start after would-be final iteration",
			prefix:        "0",
			startN:        17,
			wantHash:      "0457314be0b9283e18224b8dfad77741d9f41cdf",
			wantIteration: 43,
			wantNewCommit: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
foo 43

Message
`),
			wantOK: true,
		},
		{
			desc:          "Start at maxint iteration",
			prefix:        "1",
			startN:        math.MaxInt,
			wantHash:      "",
			wantIteration: 0,
			wantNewCommit: nil,
			wantOK:        false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			hash, iteration, newCommit, ok := find(tc.prefix, "foo", tc.startN, []byte(commit))

			if got, want := hash, tc.wantHash; got != want {
				t.Errorf("hash = %q, want %q", got, want)
			}

			if got, want := iteration, tc.wantIteration; got != want {
				t.Errorf("iteration = %d, want %d", got, want)
			}

			if !bytes.Equal(newCommit, tc.wantNewCommit) {
				t.Errorf("new commit is:\n%s\n\nwant:\n%s", newCommit, tc.wantNewCommit)
			}

			if got, want := ok, tc.wantOK; got != want {
				t.Errorf("ok = %t, want %t", got, want)
			}
		})
	}
}

func TestSHA1State(t *testing.T) {
	h1 := sha1.New()
	h2 := sha1.New()

	h1.Write([]byte("hello"))

	d1, d2 := sha1State(h1), sha1State(h2)
	*d2 = *d1

	if got, want := h2.Sum(nil), h1.Sum(nil); !bytes.Equal(got, want) {
		t.Error("hashes differ")
	}
}

func TestPaddedNSizeTailBlock(t *testing.T) {
	for messageLen := range 3 * sha1.BlockSize {
		head := []byte("foo")
		message := bytes.Repeat([]byte("x"), messageLen)

		h := sha1.New()

		nBytes := []byte("12345")

		h.Write(head)

		objectSize := len(head) + len(nBytes) + len(message)

		hashState := sha1State(h)

		nOffset := hashState.nx

		block := paddedNSizeTailBlock(hashState.x[:hashState.nx], len(nBytes), message, objectSize)

		hashState.nx = 0

		if len(block)%sha1.BlockSize != 0 {
			t.Fatalf("[%d] padded size is %d, want a multiple of %d", messageLen, len(block), sha1.BlockSize)
		}

		copy(block[nOffset:], nBytes)

		h.Write(block)

		var gotSum [sha1.Size]byte

		for i, w := range sha1State(h).h {
			binary.BigEndian.PutUint32(gotSum[i*4:], w)
		}

		wantSum := sha1.Sum(slices.Concat(head, nBytes, message))

		if gotSum != wantSum {
			t.Errorf("[%d] got %x, want %x", messageLen, gotSum, wantSum)
		}
	}
}

func TestAddToDigits(t *testing.T) {
	for n, tc := range []struct {
		digits string
		step   int
		want   string
		wantOK bool
	}{
		{"0", 1, "1", true},
		{"0", 9, "9", true},
		{"0", 10, "0", false},
		{"5", 5, "0", false},
		{"99", 1, "00", false},
		{"98", 1, "99", true},
		{"1234", 8, "1242", true},
		{"9995", 8, "0003", false},
		{"999999999", 8, "000000007", false},
		{"123456789", 8, "123456797", true},
	} {
		digits := []byte(tc.digits)

		if got, want := addToDigits(digits, tc.step), tc.wantOK; got != want {
			t.Errorf("[%d] addToDigits(%q, %d) = %t, want %t", n, tc.digits, tc.step, got, want)
		}

		if got, want := string(digits), tc.want; got != want {
			t.Errorf("[%d] digits are %q, want %q", n, got, want)
		}
	}
}

func TestHashPrefixWords(t *testing.T) {
	for n, tc := range []struct {
		prefix    string
		wantWords [5]uint32
		wantMask  [5]uint32
		wantN     int
	}{
		{"0", [5]uint32{}, [5]uint32{0xf0000000}, 1},
		{"c0ffee", [5]uint32{0xc0ffee00}, [5]uint32{0xffffff00}, 1},
		{"c0ffeebeef", [5]uint32{0xc0ffeebe, 0xef000000}, [5]uint32{0xffffffff, 0xff000000}, 2},
	} {
		words, mask, gotN := hashPrefixWords(tc.prefix)

		if words != tc.wantWords {
			t.Errorf("[%d] words = %08x, want %08x", n, words, tc.wantWords)
		}

		if mask != tc.wantMask {
			t.Errorf("[%d] mask = %08x, want %08x", n, mask, tc.wantMask)
		}

		if gotN != tc.wantN {
			t.Errorf("[%d] n = %d, want %d", n, gotN, tc.wantN)
		}
	}
}

func TestTrimHeader(t *testing.T) {
	for n, tc := range []struct {
		head   []byte
		header string
		want   []byte
	}{
		{
			head:   []byte{},
			header: "f00",
			want:   []byte{},
		},
		{
			head: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
f00 123`),
			header: "f00",
			want: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100`),
		},
		{
			head: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
f00 123`),
			header: "unknown",
			want: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
f00 123`),
		},
		{
			head: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
f00 123
committer Committer Name <committer@example.com> 1577876400 +0100`),
			header: "f00",
			want: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
f00 123
committer Committer Name <committer@example.com> 1577876400 +0100`),
		},
	} {

		got := trimHeader(tc.head, tc.header)

		if !bytes.Equal(got, tc.want) {
			t.Errorf("[%d] got:\n%s\n\nwant:\n%s", n, got, tc.want)
		}
	}
}

func TestThousandSeparate(t *testing.T) {
	for n, tc := range []struct {
		n    int
		want string
	}{
		{-1000000000, "-1,000,000,000"},
		{-100000000, "-100,000,000"},
		{-10000000, "-10,000,000"},
		{-1000000, "-1,000,000"},
		{-100000, "-100,000"},
		{-10000, "-10,000"},
		{-1000, "-1,000"},
		{-100, "-100"},
		{-10, "-10"},
		{-1, "-1"},
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{100, "100"},
		{1000, "1,000"},
		{10000, "10,000"},
		{100000, "100,000"},
		{1000000, "1,000,000"},
		{10000000, "10,000,000"},
		{100000000, "100,000,000"},
		{1000000000, "1,000,000,000"},
	} {
		if got, want := thousandSeparate(tc.n), tc.want; got != want {
			t.Errorf("[%d] thousandSeparate(%d) = %q, want %q", n, tc.n, got, want)
		}
	}
}

func BenchmarkFind(b *testing.B) {
	for b.Loop() {
		find("c0ffee", "c0ffee", 0, []byte(commit))
	}
}
