package main

import (
	"bytes"
	"testing"
)

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
	found := find("f00", "foo", []byte(commit))

	wantFound := []byte(`tree 0000000000000000000000000000000000000000
author Author Name <author@example.com> 1577872800 +0000
committer Committer Name <committer@example.com> 1577876400 +0100
foo 6113

Message
`)

	if !bytes.Equal(found, wantFound) {
		t.Errorf("got %s, want %s", found, wantFound)
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

func BenchmarkFind(b *testing.B) {
	for n := 0; n < b.N; n++ {
		find("c0ffee", "c0ffee", []byte(commit))
	}
}
