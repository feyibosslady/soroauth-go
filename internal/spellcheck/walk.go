package spellcheck

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Dictionaries are the committed files the checker is built from, relative
// to the repository root. Both are data, owned by the maintainer: the
// misspelling list grows when a real typo is caught or corrected, the word
// list when a legitimate project term shows up. See CONTRIBUTING.md,
// "Spell-checking the documentation", for the maintenance contract.
const (
	// MisspellingsFile holds "typo->correction" pairs.
	MisspellingsFile = "testdata/spell/misspellings.txt"
	// WordsFile holds project terms that must never be flagged. It is not
	// consulted by the misspelling matcher — a misspelling is a finding
	// because it is listed, full stop — but it records why each protected
	// term is protected, and New refuses a term that also sits on the
	// misspelling list, so the two files cannot contradict each other.
	WordsFile = "testdata/spell/words.txt"
)

// DocExcluded reports whether a Markdown file inside fsys is out of the
// checker's scope. Only third-party provenance prose is excluded:
//
//   - LICENSE is the upstream Apache-2.0 text and must not be edited, so a
//     typo in it can only be fixed by the Apache Software Foundation;
//   - CHANGELOG.md records history in the tense and spelling it was written
//     in, and silently rewriting old entries would falsify the record;
//   - e2e/RESULTS.md and release notes under docs/recorded are outputs of
//     tool runs and real e2e submissions.
//
// Everything else is maintained prose and is checked. The list is explicit
// rather than a directory allowlist so a new Markdown file added anywhere is
// covered by default and an exclusion has to be justified here, in code.
func DocExcluded(rel string) bool {
	switch rel {
	case "LICENSE", "CHANGELOG.md", "e2e/RESULTS.md", "docs/recorded":
		return true
	}
	return strings.HasPrefix(rel, "docs/recorded/")
}

// FindDocs returns the repository's Markdown files, sorted, excluding the
// files DocExcluded names. .markdown is treated as Markdown as well as .md;
// LICENSE has no extension and is matched by name.
func FindDocs(fsys fs.FS) ([]string, error) {
	var docs []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == ".git" || path == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name == "LICENSE" {
			// Handled below only to keep the extension rules readable.
		}
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".markdown") && name != "LICENSE" {
			return nil
		}
		rel := filepath.ToSlash(path)
		if DocExcluded(rel) {
			return nil
		}
		docs = append(docs, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(docs)
	return docs, nil
}

// WalkDocs loads the two dictionaries from the repository root dir, checks
// every Markdown file it finds, and returns all findings across all files,
// ordered by file then position.
func WalkDocs(root string) ([]Finding, error) {
	misspellData, err := os.ReadFile(filepath.Join(root, MisspellingsFile))
	if err != nil {
		return nil, err
	}
	wordsData, err := os.ReadFile(filepath.Join(root, WordsFile))
	if err != nil {
		return nil, err
	}
	misspellings, err := ParseMisspellings(misspellData)
	if err != nil {
		return nil, err
	}
	words, err := ParseWords(wordsData)
	if err != nil {
		return nil, err
	}
	checker, err := New(misspellings, words)
	if err != nil {
		return nil, err
	}

	fsys := os.DirFS(root)
	docs, err := FindDocs(fsys)
	if err != nil {
		return nil, err
	}

	var all []Finding
	for _, rel := range docs {
		content, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return nil, err
		}
		if !utf8Valid(content) {
			return nil, &NonUTF8Error{File: rel}
		}
		all = append(all, checker.Check(rel, string(content))...)
	}
	return all, nil
}

// NonUTF8Error reports a Markdown file the checker could not decode. The
// corpus is expected to be UTF-8; silently checking mis-decoded bytes would
// report positions against text no reader sees.
type NonUTF8Error struct {
	File string
}

// Error implements error.
func (e *NonUTF8Error) Error() string {
	return "soroauth: " + e.File + " is not valid UTF-8; the documentation is checked as UTF-8 text"
}

// utf8Valid reports whether b is entirely valid UTF-8.
func utf8Valid(b []byte) bool {
	return bytes.IndexFunc(b, func(r rune) bool { return r == utf8.RuneError }) < 0 && utf8.Valid(b)
}
