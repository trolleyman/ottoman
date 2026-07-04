// Command gotest-markers converts `go test -json` output into Hydra's
// streaming test markers (type = "stdout" runners in .hydra/config.toml).
//
// It reads test2json events on stdin and, for each finished test, prints one
//
//	::hydra:test:pass:: <package> › <Test> › <subtest>
//	::hydra:test:fail:: <package> › <Test> | <escaped failure output>
//	::hydra:test:skip:: <package> › <Test>
//
// line that Hydra counts live into the tests panel and the sidebar verdict
// chip. The package import path is the marker's location token (Hydra strips
// the go.mod module prefix for display); nested subtests become › scopes. A
// package that fails to build (a fail event with no Test) is surfaced as its
// own fail marker so build breaks show up as a red verdict rather than an
// empty run. The original human-readable `go test` text is passed through to
// stdout too, so the full build log stays readable.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// event mirrors the fields of a `go test -json` (test2json) record we use.
type event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	// Test output lines (panics, long diffs) can be large; grow the buffer.
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)

	// Accumulate each test's (and package's) output so a fail marker can carry
	// the failure text as its message.
	outputs := map[string][]string{}
	key := func(pkg, test string) string { return pkg + "\x00" + test }
	// Packages that had at least one failing test — their package-level FAIL is
	// then just a summary of those and must not be counted again.
	failedTest := map[string]bool{}

	for sc.Scan() {
		line := sc.Bytes()
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			// Not a JSON event (e.g. a stray build line) — pass it through.
			fmt.Println(string(line))
			continue
		}

		switch e.Action {
		case "output":
			// Echo the normal go-test text so the build log reads as usual.
			fmt.Print(e.Output)
			k := key(e.Package, e.Test)
			outputs[k] = append(outputs[k], e.Output)

		case "pass", "fail", "skip":
			k := key(e.Package, e.Test)
			if e.Test == "" {
				// Package-level result. Only a build/setup failure with no
				// failing test of its own is worth a marker; a FAIL that just
				// summarizes already-reported test failures, and pass/skip
				// ("[no test files]", per-package summaries), would double-count.
				if e.Action == "fail" && !failedTest[e.Package] {
					emit("fail", e.Package+" › (package)", failMessage(outputs[k]))
				}
				delete(failedTest, e.Package)
				delete(outputs, k)
				continue
			}
			label := e.Package + " › " + strings.ReplaceAll(e.Test, "/", " › ")
			switch e.Action {
			case "pass":
				emit("pass", label, "")
			case "skip":
				emit("skip", label, "")
			case "fail":
				failedTest[e.Package] = true
				emit("fail", label, failMessage(outputs[k]))
			}
			delete(outputs, k)
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "gotest-markers:", err)
		os.Exit(1)
	}
}

// emit prints one Hydra test marker. A non-empty message is appended after " | "
// with control characters escaped so the whole marker stays on one line.
func emit(kind, label, msg string) {
	if msg != "" {
		fmt.Printf("::hydra:test:%s:: %s | %s\n", kind, label, msg)
		return
	}
	fmt.Printf("::hydra:test:%s:: %s\n", kind, label)
}

// failMessage joins a failed test's captured output into a single escaped line,
// keeping only the tail so a long panic/diff still fits.
func failMessage(lines []string) string {
	text := strings.TrimSpace(strings.Join(lines, ""))
	if text == "" {
		return ""
	}
	const max = 1500
	if len(text) > max {
		text = text[len(text)-max:]
	}
	r := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\t", "\\t", "\r", "\\r")
	return r.Replace(text)
}
