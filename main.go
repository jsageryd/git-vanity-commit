package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

var invalidKey = regexp.MustCompile(`^(commit|tree|parent|author|committer|encoding)\b|[^a-zA-Z0-9]`).MatchString
var validPrefix = regexp.MustCompile("^[0-9a-f]{1,40}$").MatchString

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("| ")

	commit := flag.String("commit", "HEAD", "Starting point")
	prefix := flag.String("prefix", "", "Desired hash prefix (mandatory)")
	key := flag.String("key", "", "Key used in the commit header (defaults to the prefix)")
	reset := flag.Bool("reset", false, "If set, reset to the new commit (implies -write)")
	write := flag.Bool("write", false, "If set, write the new commit to the repository (hash-object -w)")
	printHash := flag.Bool("print", false, "Print the commit hash found to stdout")
	quiet := flag.Bool("quiet", false, "Suppress log output")
	startN := flag.Int("start", 0, "Iteration to start from")

	flag.Parse()

	if *prefix == "" {
		fmt.Fprintln(os.Stderr, "missing prefix")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	if !validPrefix(*prefix) {
		fmt.Fprintln(os.Stderr, "invalid prefix (must be lowercase hex)")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	if *key == "" {
		*key = *prefix
	}

	if invalidKey(*key) {
		fmt.Fprintln(os.Stderr, "invalid key")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	if *startN < 0 {
		fmt.Fprintln(os.Stderr, "starting iteration must be positive")
		os.Exit(1)
	}

	if *quiet {
		log.SetOutput(io.Discard)
	}

	commitData := fetchCommit(*commit)

	log.Printf("Using commit at %s (%s)", *commit, revParseShort(*commit))
	log.Printf("Finding hash prefixed %q", *prefix)

	ts := thousandSeparate

	log.Printf("Commit size %s bytes", ts(len(commitData)))

	if *startN > 0 {
		log.Printf("Starting at iteration %d", *startN)
	}

	start := time.Now()
	hash, iteration, newCommit := find(*prefix, *key, *startN, commitData)
	duration := time.Since(start)

	log.Printf("Tested %s commits at %s commits per second", ts((iteration - *startN + 1)), ts(int(float64(iteration-*startN+1)/duration.Seconds())))
	log.Printf("Found %s (iteration %d, %s)", hash, iteration, duration.Round(time.Millisecond))

	if *printHash {
		fmt.Println(hash)
	}

	if *write || *reset {
		writtenHash := writeCommit(newCommit)

		log.Println("Commit object written")

		if hash != writtenHash {
			fmt.Printf("hash mismatch: git-vanity-commit %q vs. hash-object output %q\n", hash, writtenHash)
			os.Exit(1)
		}
	}

	if *reset {
		resetTo(hash)
		log.Printf("HEAD is now at %s", hash)
	}
}

func revParseShort(rev string) string {
	out, err := exec.Command("git", "rev-parse", "--short=12", "--verify", rev).Output()
	if err != nil {
		if eErr, ok := err.(*exec.ExitError); ok {
			log.Fatalf("error parsing revision; git says %v", string(eErr.Stderr))
		} else {
			log.Fatalf("error parsing revision: %v", err)
		}
	}
	return string(bytes.TrimSpace(out))
}

func fetchCommit(ref string) []byte {
	shortRef := revParseShort(ref)

	out, err := exec.Command("git", "cat-file", "-t", ref).Output()
	if err != nil {
		if eErr, ok := err.(*exec.ExitError); ok {
			log.Fatalf("error reading object type; git says %v", string(eErr.Stderr))
		} else {
			log.Fatalf("error reading object type: %v", err)
		}
	}
	if got, want := strings.TrimSpace(string(out)), "commit"; got != want {
		log.Fatalf("%s is a %s object; expected a commit", shortRef, got)
	}

	out, err = exec.Command("git", "cat-file", "commit", ref).Output()
	if err != nil {
		if eErr, ok := err.(*exec.ExitError); ok {
			log.Fatalf("error reading commit; git says %v", string(eErr.Stderr))
		} else {
			log.Fatalf("error reading commit: %v", err)
		}
	}
	return out
}

func find(hashPrefix, header string, startN int, commit []byte) (hash string, iteration int, newCommit []byte) {
	done := make(chan struct{})

	type res struct {
		hash string
		n    int
		b    []byte
	}

	found := make(chan res)

	var firstN int

	var wg sync.WaitGroup

	work := func(offset, stepSize int) {
		defer wg.Done()

		h := sha1.New()
		head, tail := headTail(commit)
		head = trimHeader(head, header)

		scratch := make([]byte, 0, sha1.Size)

		commitHeaderBytes := []byte("commit ")
		headerBytes := []byte("\n" + header + " ")
		nullByte := []byte{0x00}

		hashMask := byte(0xff)
		var suffix string

		if len(hashPrefix)%2 != 0 {
			hashMask = 0xf0
			suffix = "0"
		}

		hashPrefixBytes, _ := hex.DecodeString(hashPrefix + suffix)

		var nBytes []byte
		var commitSizeBytes []byte

		var lastCommitSize int
		lastH := sha1.New()

		for n := offset; ; n += stepSize {
			nBytes = strconv.AppendInt(nBytes[:0], int64(n), 10)
			commitSize := len(head) + len(tail) + len(header) + 1 + len(nBytes) + 1
			if lastCommitSize != commitSize {
				h.Reset()
				commitSizeBytes = strconv.AppendInt(commitSizeBytes[:0], int64(commitSize), 10)
				h.Write(commitHeaderBytes)
				h.Write(commitSizeBytes)
				h.Write(nullByte)
				h.Write(head)
				h.Write(headerBytes)
				copySHA1Hash(lastH, h)
				lastCommitSize = commitSize
			}
			copySHA1Hash(h, lastH)
			h.Write(nBytes)
			h.Write(tail)
			candidate := h.Sum(scratch[:0])
			if bytes.HasPrefix(candidate, hashPrefixBytes[:len(hashPrefixBytes)-1]) &&
				candidate[len(hashPrefixBytes)-1]&hashMask == hashPrefixBytes[len(hashPrefixBytes)-1]&hashMask {
				buf := new(bytes.Buffer)
				buf.Write(head)
				buf.Write(headerBytes)
				buf.Write(nBytes)
				buf.Write(tail)
				found <- res{hex.EncodeToString(candidate), n, buf.Bytes()}
				return
			}

			select {
			case <-done:
				if n > firstN {
					return
				}
			default:
			}
		}
	}

	workers := runtime.GOMAXPROCS(0)

	if numCPU := runtime.NumCPU(); workers > numCPU {
		workers = numCPU
	}

	wg.Add(workers)

	log.Printf("Using %d concurrent workers", workers)

	for i := startN; i < startN+workers; i++ {
		go work(i, workers)
	}

	go func() {
		wg.Wait()
		close(found)
	}()

	minRes := <-found
	firstN = minRes.n

	close(done)

	for r := range found {
		if r.n < minRes.n {
			minRes = r
		}
	}

	return minRes.hash, minRes.n, minRes.b
}

func copySHA1Hash(dst, src hash.Hash) {
	type eface struct {
		_type uintptr
		data  unsafe.Pointer
	}

	type digest struct {
		h   [5]uint32
		x   [64]byte
		nx  int
		len uint64
	}

	*(*digest)((*eface)(unsafe.Pointer(&dst)).data) = *(*digest)((*eface)(unsafe.Pointer(&src)).data)
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
		if eErr, ok := err.(*exec.ExitError); ok {
			log.Fatalf("error writing object; git says %v", string(eErr.Stderr))
		} else {
			log.Fatalf("error writing object: %v", err)
		}
	}

	return string(bytes.TrimSpace(out))
}

func resetTo(hash string) {
	if err := exec.Command("git", "reset", hash).Run(); err != nil {
		if eErr, ok := err.(*exec.ExitError); ok {
			log.Fatalf("error resetting to commit; git says %v", string(eErr.Stderr))
		} else {
			log.Fatalf("error resettting to commit: %v", err)
		}
	}
}

func thousandSeparate(n int) string {
	var newS string

	if n < 0 {
		n = -n
		newS = "-"
	}

	s := strconv.Itoa(n)

	for n := range s {
		if n != 0 && n%3 == len(s)%3 {
			newS += ","
		}

		newS += string(s[n])
	}

	return newS
}
