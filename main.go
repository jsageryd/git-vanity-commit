package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("| ")

	commit := flag.String("commit", "HEAD", "Starting point")
	prefix := flag.String("prefix", "", "Desired hash prefix (mandatory)")
	reset := flag.Bool("reset", false, "If set, reset to the new commit")

	flag.Parse()

	if *prefix == "" {
		fmt.Fprintln(os.Stderr, "missing prefix")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	log.Printf("Using commit at %s (%s)", *commit, revParseShort(*commit))
	log.Printf("Finding hash prefixed %q", *prefix)

	start := time.Now()

	headCommit := fetchCommit(*commit)
	newCommit := find(*prefix, *prefix, headCommit)
	hash := writeCommit(newCommit)

	log.Printf("Found %s (%s)", hash, time.Since(start).Round(time.Millisecond))

	if *reset {
		resetTo(hash)
		log.Printf("HEAD is now at %s", hash)
	}
}

func revParseShort(rev string) string {
	out, err := exec.Command("git", "rev-parse", "--short=12", "--verify", rev).Output()
	if err != nil {
		log.Fatalf("cannot find commit: %v", err)
	}
	return string(bytes.TrimSpace(out))
}

func fetchCommit(ref string) []byte {
	out, err := exec.Command("git", "cat-file", "-p", ref).Output()
	if err != nil {
		log.Fatal(err)
	}
	return out
}

func find(hashPrefix, header string, commit []byte) []byte {
	h := sha1.New()
	head, tail := headTail(commit)

	dst := make([]byte, sha1.Size*2)

	for n := 0; ; n++ {
		fmt.Fprintf(h, "commit %d\x00", len(commit)+len(header)+1+len(strconv.Itoa(n))+1)
		h.Write(head)
		fmt.Fprintf(h, "\n%s %d", header, n)
		h.Write(tail)
		candidate := h.Sum(nil)
		hex.Encode(dst, candidate)
		if bytes.Equal(dst[:len(hashPrefix)], []byte(hashPrefix)) {
			buf := new(bytes.Buffer)
			buf.Write(head)
			fmt.Fprintf(buf, "\n%s %d", header, n)
			buf.Write(tail)
			return buf.Bytes()
		}
		h.Reset()
	}
}

func headTail(commit []byte) (head, tail []byte) {
	idx := bytes.Index(commit, []byte("\n\n"))
	if idx == -1 {
		log.Fatal("cannot parse commit")
	}
	return commit[:idx], commit[idx:]
}

func writeCommit(commit []byte) (hash string) {
	cmd := exec.Command("git", "hash-object", "--stdin", "-t", "commit", "-w")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		stdin.Write(commit)
		stdin.Close()
	}()

	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	}

	return string(bytes.TrimSpace(out))
}

func resetTo(hash string) {
	if err := exec.Command("git", "reset", hash).Run(); err != nil {
		log.Fatal(err)
	}
}
