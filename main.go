package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
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

	hash, iteration, newCommit, ok := find(*prefix, *key, *startN, commitData)
	if !ok {
		log.Println("No hash found")
		os.Exit(1)
	}

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

func find(hashPrefix, header string, startN int, commit []byte) (hash string, iteration int, newCommit []byte, ok bool) {
	const pollInterval = 256

	done := make(chan struct{})

	type res struct {
		hash string
		n    int
		b    []byte
	}

	found := make(chan res)

	var firstN int

	var wg sync.WaitGroup

	prefixWords, prefixMask, prefixLen := hashPrefixWords(hashPrefix)

	work := func(offset, stepSize int) {
		defer wg.Done()

		h := sha1.New()
		hashState := sha1State(h)

		head, tail := headTail(commit)
		head = trimHeader(head, header)

		commitHeaderBytes := []byte("commit ")
		headerBytes := []byte("\n" + header + " ")
		nullByte := []byte{0x00}

		var nBytes []byte
		var commitSizeBytes []byte

		var lastSum [5]uint32

		var nBytesTailAndPadding []byte

		for n, poll := offset, 0; n >= 0; n += stepSize {
			if !addToDigits(nBytes, stepSize) {
				nBytes = strconv.AppendInt(nBytes[:0], int64(n), 10)
				commitSize := len(head) + len(tail) + len(header) + 1 + len(nBytes) + 1
				h.Reset()
				commitSizeBytes = strconv.AppendInt(commitSizeBytes[:0], int64(commitSize), 10)
				h.Write(commitHeaderBytes)
				h.Write(commitSizeBytes)
				h.Write(nullByte)
				h.Write(head)
				h.Write(headerBytes)
				lastSum = hashState.h

				objectSize := len(commitHeaderBytes) + len(commitSizeBytes) + len(nullByte) + commitSize

				nOffset := hashState.nx
				nBytesTailAndPadding = paddedNSizeTailBlock(hashState.x[:nOffset], len(nBytes), tail, objectSize)
				copy(nBytesTailAndPadding[nOffset:], nBytes)
				nBytes = nBytesTailAndPadding[nOffset : nOffset+len(nBytes)]

				hashState.nx = 0
			}
			hashState.h = lastSum
			h.Write(nBytesTailAndPadding)

			if hashState.h[0]&prefixMask[0] == prefixWords[0] && match(&hashState.h, &prefixWords, &prefixMask, prefixLen) {
				var sum [sha1.Size]byte
				for i, w := range hashState.h {
					binary.BigEndian.PutUint32(sum[i*4:], w)
				}

				buf := new(bytes.Buffer)
				buf.Write(head)
				buf.Write(headerBytes)
				buf.Write(nBytes)
				buf.Write(tail)
				found <- res{hex.EncodeToString(sum[:]), n, buf.Bytes()}
				return
			}

			if poll++; poll == pollInterval {
				poll = 0

				select {
				case <-done:
					if n > firstN {
						return
					}
				default:
				}
			}
		}
	}

	workers := runtime.GOMAXPROCS(0)

	if numCPU := runtime.NumCPU(); workers > numCPU {
		workers = numCPU
	}

	log.Printf("Using %d concurrent workers", workers)

	for i := range workers {
		offset := startN + i
		if offset < 0 {
			break
		}
		wg.Add(1)
		go work(offset, workers)
	}

	go func() {
		wg.Wait()
		close(found)
	}()

	minRes, ok := <-found
	firstN = minRes.n

	close(done)

	for r := range found {
		if r.n < minRes.n {
			minRes = r
		}
	}

	return minRes.hash, minRes.n, minRes.b, ok
}

type sha1Digest struct {
	h   [5]uint32
	x   [64]byte
	nx  int
	len uint64
}

func sha1State(h hash.Hash) *sha1Digest {
	type eface struct {
		_type uintptr
		data  unsafe.Pointer
	}

	return (*sha1Digest)((*eface)(unsafe.Pointer(&h)).data)
}

// hashPrefixWords returns the desired hash prefix as big-endian words, the
// mask selecting the significant bits of each, and the number of words used.
func hashPrefixWords(hashPrefix string) (words, mask [5]uint32, n int) {
	padded, _ := hex.DecodeString(hashPrefix + strings.Repeat("0", sha1.Size*2-len(hashPrefix)))

	bits := 4 * len(hashPrefix)

	for i := range words {
		words[i] = binary.BigEndian.Uint32(padded[i*4:])

		if b := bits - 32*i; b > 0 {
			if b > 32 {
				b = 32
			}
			mask[i] = ^uint32(0) << (32 - b)
			words[i] &= mask[i]
			n = i + 1
		}
	}

	return words, mask, n
}

func match(sum, words, mask *[5]uint32, n int) bool {
	for i := range n {
		if sum[i]&mask[i] != words[i] {
			return false
		}
	}
	return true
}

// addToDigits adds step to the decimal digits in place. It reports whether the
// result still fits in the same number of digits.
func addToDigits(digits []byte, step int) bool {
	carry := step

	for i := len(digits) - 1; i >= 0; i-- {
		if carry == 0 {
			return true
		}
		v := int(digits[i]-'0') + carry
		digits[i] = byte('0' + v%10)
		carry = v / 10
	}

	return carry == 0
}

// paddedNSizeTailBlock returns a buffer that starts with the given already
// buffered bytes, leaves nLen bytes for the caller to fill in the nonce, and
// ends with the given tail and the SHA-1 padding, on a block boundary.
func paddedNSizeTailBlock(buffered []byte, nLen int, tail []byte, objectSize int) []byte {
	size := len(buffered) + nLen + len(tail) + 1 + 8

	if r := size % sha1.BlockSize; r != 0 {
		size += sha1.BlockSize - r
	}

	block := make([]byte, size)
	nOffset := copy(block, buffered)
	copy(block[nOffset+nLen:], tail)
	block[nOffset+nLen+len(tail)] = 0x80
	binary.BigEndian.PutUint64(block[size-8:], uint64(objectSize)*8)

	return block
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
