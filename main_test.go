package main

import (
	"bytes"
	"io"
	"log"
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
		wantHash      string
		wantIteration int
		wantNewCommit []byte
	}{
		{
			desc:          "Start at 0th iteration",
			startN:        0,
			wantHash:      "034c4f788c4a7522a75e1b86ee3c24eee630e822",
			wantIteration: 16,
			wantNewCommit: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
foo 16

Message
`),
		},
		{
			desc:          "Start at final iteration",
			startN:        16,
			wantHash:      "034c4f788c4a7522a75e1b86ee3c24eee630e822",
			wantIteration: 16,
			wantNewCommit: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
foo 16

Message
`),
		},
		{
			desc:          "Start after would-be final iteration",
			startN:        17,
			wantHash:      "0457314be0b9283e18224b8dfad77741d9f41cdf",
			wantIteration: 43,
			wantNewCommit: []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
foo 43

Message
`),
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			hash, iteration, newCommit := find("0", "foo", tc.startN, []byte(commit))

			if got, want := hash, tc.wantHash; got != want {
				t.Errorf("hash = %q, want %q", got, want)
			}

			if got, want := iteration, tc.wantIteration; got != want {
				t.Errorf("iteration = %d, want %d", got, want)
			}

			if !bytes.Equal(newCommit, tc.wantNewCommit) {
				t.Errorf("new commit is:\n%s\n\nwant:\n%s", newCommit, tc.wantNewCommit)
			}
		})
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
