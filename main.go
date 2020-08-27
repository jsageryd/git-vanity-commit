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
	"regexp"
	"runtime"
	"strconv"
	"time"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("| ")

	commit := flag.String("commit", "HEAD", "Starting point")
	prefix := flag.String("prefix", "", "Desired hash prefix (mandatory)")
	key := flag.String("key", "", "Key used in the commit header (defaults to the prefix)")
	reset := flag.Bool("reset", false, "If set, reset to the new commit")

	flag.Parse()

	if *prefix == "" {
		fmt.Fprintln(os.Stderr, "missing prefix")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	validPrefix := regexp.MustCompile("^[0-9a-f]{0,40}$").MatchString
	if !validPrefix(*prefix) {
		fmt.Fprintln(os.Stderr, "invalid prefix (must be lowercase hex)")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	if *key == "" {
		*key = *prefix
	}

	invalidKey := regexp.MustCompile("^(commit|tree|parent|author|committer|encoding|[^a-zA-Z0-9])$").MatchString
	if invalidKey(*key) {
		fmt.Fprintln(os.Stderr, "invalid key")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	log.Printf("Using commit at %s (%s)", *commit, revParseShort(*commit))
	log.Printf("Finding hash prefixed %q", *prefix)

	start := time.Now()

	headCommit := fetchCommit(*commit)
	newCommit := find(*prefix, *key, headCommit)
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
	intSlices := make(chan []int, 2048)
	done := make(chan struct{})
	found := make(chan []byte)

	work := func() {
		h := sha1.New()
		head, tail := headTail(commit)
		head = trimHeader(head, header)

		dst := make([]byte, sha1.Size*2)
		scratch := make([]byte, 0, sha1.Size)

		for intSlice := range intSlices {
			for _, n := range intSlice {
				fmt.Fprintf(h, "commit %d\x00", len(head)+len(tail)+len(header)+1+len(strconv.Itoa(n))+1)
				h.Write(head)
				fmt.Fprintf(h, "\n%s %d", header, n)
				h.Write(tail)
				candidate := h.Sum(scratch[:0])
				hex.Encode(dst, candidate)
				if bytes.Equal(dst[:len(hashPrefix)], []byte(hashPrefix)) {
					buf := new(bytes.Buffer)
					buf.Write(head)
					fmt.Fprintf(buf, "\n%s %d", header, n)
					buf.Write(tail)
					found <- buf.Bytes()
					close(done)
					return
				}
				h.Reset()
			}
		}
	}

	workers := runtime.NumCPU()
	log.Printf("Using %d concurrent workers", workers)

	for i := 0; i < workers; i++ {
		go work()
	}

	const intSliceSize = 100

	go func() {
		intSlice := make([]int, 0, intSliceSize)
		for n := 0; ; n++ {
			intSlice = append(intSlice, n)

			if n%intSliceSize == 0 {
				select {
				case <-done:
					close(intSlices)
					return
				default:
					intSlices <- intSlice
				}

				intSlice = make([]int, 0, intSliceSize)
			}
		}
	}()

	return <-found
}

func headTail(commit []byte) (head, tail []byte) {
	idx := bytes.Index(commit, []byte("\n\n"))
	if idx == -1 {
		log.Fatal("cannot parse commit")
	}
	return commit[:idx], commit[idx:]
}

func trimHeader(head []byte, header string) []byte {
	idx := bytes.LastIndex(head, []byte("\n"))
	if idx == -1 {
		return head
	}

	if bytes.HasPrefix(head[idx+1:], []byte(header)) {
		return head[:idx]
	}

	return head
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
