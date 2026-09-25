// Command spellcheck fails when the repository's Markdown prose holds a
// known misspelling, a doubled English function word, or a byte sequence
// that is not valid UTF-8.
//
// It is the CLI form of internal/spellcheck: the library holds the rules
// and the two committed dictionaries (testdata/spell/misspellings.txt,
// testdata/spell/words.txt), this command walks the documentation corpus,
// prints one line per finding, and exits non-zero if there are any. The
// root test TestDocumentationProseIsClean runs the same walk, so `go test
// ./...` gates the docs even where this command is not installed.
//
// Files that are recorded provenance rather than maintained prose are
// excluded by internal/spellcheck.DocExcluded (LICENSE, CHANGELOG.md,
// e2e/RESULTS.md, docs/recorded/): their text is history or third-party
// material and is not edited to satisfy a checker.
//
// Usage:
//
//	go run ./scripts/spellcheck                 # check the repo's docs
//	go run ./scripts/spellcheck -v              # also print the per-file summary
//
// Exit status: 0 no findings, 1 findings or a dictionary error, 2 usage or
// read error.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/soroauth/soroauth-go/internal/spellcheck"
)

func main() {
	verbose := flag.Bool("v", false, "print a per-file summary of what was checked")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: spellcheck [-v]\n\n"+
			"Checks every Markdown file in the repository (from the working directory)\n"+
			"against testdata/spell/misspellings.txt and testdata/spell/words.txt.\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	findings, err := spellcheck.WalkDocs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "spellcheck: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		docs, err := spellcheck.FindDocs(os.DirFS("."))
		if err != nil {
			fmt.Fprintf(os.Stderr, "spellcheck: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("checked %d files\n", len(docs))
		checked := make(map[string]bool, len(docs))
		for _, d := range docs {
			checked[d] = true
		}
		perFile := make(map[string]int)
		for _, f := range findings {
			perFile[f.File]++
		}
		for _, d := range docs {
			if n := perFile[d]; n > 0 {
				fmt.Printf("  %s: %d findings\n", d, n)
			}
		}
	}

	for _, f := range findings {
		switch f.Rule {
		case spellcheck.RuleMisspelling:
			fmt.Printf("%s:%d:%d: %q is a known misspelling of %q\n", f.File, f.Line, f.Col, f.Word, f.Fix)
		case spellcheck.RuleDoubled:
			fmt.Printf("%s:%d:%d: %q repeats a word\n", f.File, f.Line, f.Col, f.Word)
		default:
			fmt.Printf("%s:%d:%d: %q [%s]\n", f.File, f.Line, f.Col, f.Word, f.Rule)
		}
	}

	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "\nspellcheck: %d findings. Fix the prose, or, if a word is a legitimate "+
			"project term the checker should never flag, document it in testdata/spell/words.txt; "+
			"see CONTRIBUTING.md, \"Spell-checking the documentation\".\n", len(findings))
		os.Exit(1)
	}
	if *verbose {
		fmt.Println("spellcheck: no findings")
	}
}
