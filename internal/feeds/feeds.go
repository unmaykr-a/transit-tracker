// Package feeds parses the upstream transit feeds documented in docs/feeds.md.
//
// Parsers are pure: they take an io.Reader and never touch the network, so every
// format can be tested against the recorded fixtures in testdata/.
package feeds

import (
	"bufio"
	"io"
	"strings"
)

// Mode is a transport mode, shared by every feed.
type Mode string

const (
	ModeTram    Mode = "tram"
	ModeBus     Mode = "bus"
	ModeTrolley Mode = "trolley"
	ModeTrain   Mode = "train"
)

// lines yields the non-empty lines of r with the UTF-8 BOM and CR stripped.
func lines(r io.Reader, fn func(line string)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	n := 0
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if n == 0 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		n++
		if strings.TrimSpace(line) == "" {
			continue
		}
		fn(line)
	}
	return sc.Err()
}
