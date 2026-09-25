package spellcheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testChecker builds a Checker with a small, explicit dictionary. Tests
// that need specific rows inline them; the committed dictionaries are
// exercised separately by the file-format tests.
func testChecker(t *testing.T, misspellings map[string]string, allowed map[string]bool) *Checker {
	t.Helper()
	c, err := New(misspellings, allowed)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestCheckFindsListedMisspellings asserts the basic contract: a listed
// typo is found, at the right file/line/column, with the committed
// correction, regardless of capitalization.
func TestCheckFindsListedMisspellings(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)

	tests := []struct {
		name    string
		content string
		want    []Finding
	}{
		{
			name:    "lowercase",
			content: "you will recieve the entry\n",
			want: []Finding{
				{File: "f.md", Line: 1, Col: 10, Word: "recieve", Rule: RuleMisspelling, Fix: "receive"},
			},
		},
		{
			name:    "sentence case",
			content: "Recieve the entry\n",
			want: []Finding{
				{File: "f.md", Line: 1, Col: 1, Word: "Recieve", Rule: RuleMisspelling, Fix: "receive"},
			},
		},
		{
			name:    "all caps",
			content: "RECIEVE it\n",
			want: []Finding{
				{File: "f.md", Line: 1, Col: 1, Word: "RECIEVE", Rule: RuleMisspelling, Fix: "receive"},
			},
		},
		{
			name:    "second line column is per-line",
			content: "first line is fine\nit will recieve a signature\n",
			want: []Finding{
				{File: "f.md", Line: 2, Col: 9, Word: "recieve", Rule: RuleMisspelling, Fix: "receive"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Check("f.md", tt.content)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d findings, want %d:\n%s", len(got), len(tt.want), formatFindings(got))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("finding %d:\n got %+v\nwant %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCheckSplitsCamelCaseBoundaries asserts a typo glued into an
// identifier-shaped word is still seen, and that acronym runs are not.
func TestCheckSplitsCamelCaseBoundaries(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)

	got := c.Check("f.md", "the recieveSomething helper is wrong\n")
	want := []Finding{
		{File: "f.md", Line: 1, Col: 5, Word: "recieve", Rule: RuleMisspelling, Fix: "receive"},
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got %s\nwant %s", formatFindings(got), formatFindings(want))
	}

	// "XDRPayload" has no lower→upper boundary (XDR is all caps), so it
	// stays whole and, being unlisted, is not a finding.
	if got := c.Check("f.md", "decode the XDRPayload\n"); len(got) != 0 {
		t.Errorf("XDRPayload produced findings: %s", formatFindings(got))
	}
}

// TestCheckSkipsNonProse asserts code, identifiers and URLs are never
// checked. Every content line here holds the same "recieve" typo in a
// non-prose position; a checker that reports any of them would fail CI on
// code that is not English.
func TestCheckSkipsNonProse(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "fenced code block",
			content: "prose is fine\n\n```go\nsig := recieve(payload)\n```\n\nmore prose\n",
		},
		{
			name:    "fenced code with info string",
			content: "```sh\n# recieve happens here\ngo test ./...\n```\n",
		},
		{
			name:    "tilde fence",
			content: "~~~\nrecieve\n~~~\n",
		},
		{
			name:    "inline code span",
			content: "the `recieve(payload)` call and `xdr.Recieve` type\n",
		},
		{
			name:    "double backtick span holding backtick-like text",
			content: "the ``a ` recieve `` span\n",
		},
		{
			name:    "inline link target",
			content: "see [the guide](https://example.com/recieve) for prose\n",
		},
		{
			name:    "bare autolink",
			content: "docs at <https://example.com/recieve> explain\n",
		},
		{
			name:    "bare url in text",
			content: "fetch https://example.com/recieve?from=1 for the data\n",
		},
		{
			name:    "reference link label",
			content: "the [guide][recieve-ref] covers it\n\n[recieve-ref]: https://example.com/recieve\n",
		},
		{
			name:    "html comment",
			content: "prose <!-- recieve this comment --> and prose\n",
		},
		{
			name:    "unclosed html comment blanks to end",
			content: "prose <!-- recieve\n",
		},
		{
			name:    "url with parens",
			content: "the page (https://en.wikipedia.org/wiki/Recieve_(disambiguation)) mentions\n",
		},
		{
			name:    "trailing punctuation after code span",
			content: "use `soroauth sign`, `recieve` is not a command.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Check("f.md", tt.content); len(got) != 0 {
				t.Errorf("non-prose was checked: %s", formatFindings(got))
			}
		})
	}
}

// TestCheckFindsProseAroundNonProse asserts the blanking is surgical: prose
// on the same line as a skipped construct is still checked, at the column
// the original line shows.
func TestCheckFindsProseAroundNonProse(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)

	tests := []struct {
		name    string
		content string
		wantCol int
	}{
		{name: "before inline code", content: "we recieve `payload` bytes\n", wantCol: 4},
		{name: "after inline code", content: "the `sig` value recieve errors\n", wantCol: 17},
		{name: "before a link", content: "you recieve [docs](https://x.example) now\n", wantCol: 5},
		{name: "after a bare url", content: "read https://x.example/guide and recieve it\n", wantCol: 34},
		{name: "link text stays prose", content: "here is [how to recieve](docs.md) the entry\n", wantCol: 17},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Check("f.md", tt.content)
			if len(got) != 1 || got[0].Col != tt.wantCol {
				t.Fatalf("got %s\nwant one finding at col %d", formatFindings(got), tt.wantCol)
			}
		})
	}
}

// TestCheckUTF8ColumnsAreRuneCounts asserts a multibyte character before a
// finding does not shift the reported column: columns count runes, the way
// an editor does.
func TestCheckUTF8ColumnsAreRuneCounts(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)

	// "…" is one rune, three bytes. Byte columns would report 7; rune
	// columns also report 7 here — the case that shows the difference is a
	// multibyte char *before* the column, e.g. "… the recieve" would be
	// byte-column 12 but rune-column 10.
	got := c.Check("f.md", "the … recieve\n")
	if len(got) != 1 || got[0].Col != 7 {
		t.Fatalf("got %s, want one finding at rune column 7", formatFindings(got))
	}
	got = c.Check("f.md", "… the recieve\n")
	// Byte column would be 9 here (3 bytes for …, then " the " = 5); the
	// rune column is 7.
	if len(got) != 1 || got[0].Col != 7 {
		t.Fatalf("got %s, want one finding at rune column 7", formatFindings(got))
	}
}

// TestCheckDoubledWords asserts the doubled-word rule fires on English
// function words and only across a blank gap.
func TestCheckDoubledWords(t *testing.T) {
	c := testChecker(t, nil, nil)

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "the the", content: "sign the the entry\n", want: true},
		{name: "of of", content: "a pair of of entries\n", want: true},
		{name: "capitalized repeat", content: "The the guard bites\n", want: true},
		{name: "two occurrences on one line", content: "an an entry and of of nodes\n", want: true},
		{name: "punctuation between", content: "sign the, the entry\n", want: false},
		{name: "hyphen between", content: "the-the construction\n", want: false},
		{name: "glued camel", content: "theThe entry\n", want: false},
		{name: "different words", content: "the entry\n", want: false},
		{name: "repeat allowed by project word", content: "via via delegation\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c2 *Checker
			if tt.name == "repeat allowed by project word" {
				c2 = testChecker(t, nil, map[string]bool{"via": true})
			} else {
				c2 = c
			}
			got := c2.Check("f.md", tt.content)
			if tt.want && len(got) == 0 {
				t.Fatalf("expected a doubled-word finding, got none")
			}
			if !tt.want && len(got) != 0 {
				t.Fatalf("unexpected findings: %s", formatFindings(got))
			}
			if tt.want {
				if got[0].Rule != RuleDoubled {
					t.Errorf("rule = %q, want %q", got[0].Rule, RuleDoubled)
				}
				if got[0].Fix != "" {
					t.Errorf("doubled-word finding carries a fix: %+v", got[0])
				}
			}
		})
	}
}

// TestDoubledRuleExcludesGrammaticalRepeats pins the words deliberately
// excluded from the doubled rule: their repetition can be correct prose,
// and flagging it would train contributors to ignore the check.
func TestDoubledRuleExcludesGrammaticalRepeats(t *testing.T) {
	c := testChecker(t, nil, nil)
	for _, word := range []string{"that", "had", "very"} {
		if doubledWords[word] {
			t.Errorf("%q is in doubledWords; its repeat can be grammatical", word)
		}
		if got := c.Check("f.md", "we saw "+word+" "+word+" yesterday\n"); len(got) != 0 {
			t.Errorf("%q %q produced: %s", word, word, formatFindings(got))
		}
	}
	// "no no" and "more more" are flagged: their repeats read as typos.
	for _, word := range []string{"no", "more"} {
		if got := c.Check("f.md", "we saw "+word+" "+word+" yesterday\n"); len(got) == 0 {
			t.Errorf("%q %q was not flagged", word, word)
		}
	}
}

// TestCheckIgnoresProtocolTerms is the acceptance-criteria test: the
// protocol vocabulary this project lives on must never produce a finding
// in any spelling or embedding.
func TestCheckIgnoresProtocolTerms(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)

	// None of these lines contain a listed misspelling or a doubled function
	// word, so a protocol term being flagged would mean the checker matches
	// unlisted words — the design rejects that.
	for _, line := range []string{
		"Soroban is the smart contracts platform",
		"The CAP-71-01 tree nests delegates arbitrarily",
		"CAP-46-11 covers source-account credentials",
		"XDR encoding, base64 over the wire",
		"ed25519 signatures, SHA-256 payloads",
		"a strkey is a G… or C… address",
		"SorobanAuthorizationEntry and HashIdPreimage",
		"require_auth and __check_auth",
		"ENVELOPE_TYPE_SOROBAN_AUTHORIZATION (9)",
		"SOROBAN_CREDENTIALS_ADDRESS_V2",
		"validUntilLedger and signatureExpirationLedger",
		"ScvVoid, ScvVec, ScvMap, ScvBytes",
		"friendbot funds the testnet account",
		"the stellar-cli builds to wasm32v1-none",
	} {
		t.Run(line, func(t *testing.T) {
			if got := c.Check("f.md", line+"\n"); len(got) != 0 {
				t.Errorf("protocol line produced findings: %s", formatFindings(got))
			}
		})
	}
}

// TestCheckFindsMisspellingInsideProtocolLine asserts the same lines do not
// become a blind spot: a listed typo sitting next to protocol vocabulary is
// still caught.
func TestCheckFindsMisspellingInsideProtocolLine(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)
	got := c.Check("f.md", "the SorobanAuthorizationEntry you recieve\n")
	if len(got) != 1 || got[0].Word != "recieve" {
		t.Fatalf("got %s, want one recieve finding", formatFindings(got))
	}
}

// TestCheckMatchesInflectedMisspellings asserts prose inflections of a
// listed typo are caught, with the suggestion inflected back to the correct
// spelling. Prose does not only use citation forms; a matcher that flagged
// "recieve" but waved through "recieves" would be a gate with a door in it.
func TestCheckMatchesInflectedMisspellings(t *testing.T) {
	c := testChecker(t, map[string]string{
		"recieve":    "receive",
		"occured":    "occurred",
		"propogate":  "propagate",
		"definately": "definitely",
	}, nil)

	tests := []struct {
		name     string
		content  string
		wantWord string
		wantFix  string
	}{
		{name: "third person s", content: "the node recieves the entry\n", wantWord: "recieves", wantFix: "receives"},
		{name: "past tense ed", content: "the failure occured during signing\n", wantWord: "occured", wantFix: "occurred"},
		{name: "ing form", content: "propogating the change is wrong\n", wantWord: "propogating", wantFix: "propagating"},
		{name: "e-dropping ing", content: "recieving the entry is fine prose, badly spelled\n", wantWord: "recieving", wantFix: "receiving"},
		{name: "ly adverb", content: "it definately fails\n", wantWord: "definately", wantFix: "definitely"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Check("f.md", tt.content)
			if len(got) != 1 || got[0].Word != tt.wantWord || got[0].Fix != tt.wantFix {
				t.Fatalf("got %s, want one %q finding with fix %q", formatFindings(got), tt.wantWord, tt.wantFix)
			}
		})
	}
}

// TestCheckInflectionCannotInventFindings asserts stripping extends only a
// listed row: ordinary words whose stripped stems are unlisted are never
// findings, whatever suffix they carry.
func TestCheckInflectionCannotInventFindings(t *testing.T) {
	c := testChecker(t, map[string]string{"recieve": "receive"}, nil)
	for _, line := range []string{
		"crosses and cactus and status stay\n", // stripped stems unlisted
		"the address receives a signature\n",   // "address"+es -> "address" unlisted
		"processing takes a pass\n",            // "process"+ing... "process" is not a typo row
		"class dismissed us\n",                 // double-s stems never match
	} {
		if got := c.Check("f.md", line); len(got) != 0 {
			t.Errorf("inflection invented a finding: %s", formatFindings(got))
		}
	}
}

// TestParseMisspellings covers the dictionary file format, including the
// contradictions it must refuse.
func TestParseMisspellings(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
		wantErr string
	}{
		{
			name:    "valid",
			content: "# comment\n\nrecieve->receive\n SEPERATE -> separate \n",
			want:    map[string]string{"recieve": "receive", "seperate": "separate"},
		},
		{
			name:    "uppercase typo is passed through lowercased",
			content: "Recieve->receive\n",
			want:    map[string]string{"recieve": "receive"},
		},
		{
			name:    "duplicate identical rows are tolerated",
			content: "recieve->receive\nrecieve->receive\n",
			want:    map[string]string{"recieve": "receive"},
		},
		{
			name:    "duplicate conflicting rows rejected",
			content: "recieve->receive\nrecieve->get\n",
			wantErr: "listed twice with different corrections",
		},
		{
			name:    "missing separator rejected",
			content: "recieve receive\n",
			wantErr: "no -> separator",
		},
		{
			name:    "empty correction rejected",
			content: "recieve->\n",
			wantErr: "has an empty correction",
		},
		{
			name:    "self-correction rejected",
			content: "recieve->recieve\n",
			wantErr: "corrects to itself",
		},
		{
			name:    "entry with digits rejected",
			content: "sha2->sha\n",
			wantErr: "not a lowercase alphabetic token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMisspellings([]byte(tt.content))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d rows, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("row %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// TestParseWords covers the project-words file format.
func TestParseWords(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
		wantErr string
	}{
		{name: "valid", content: "# c\n\nsoroban\nstrkey\n", want: 2},
		{name: "duplicates rejected", content: "soroban\nsoroban\n", wantErr: "listed twice"},
		{name: "uppercase is lowercased on read", content: "Soroban\n", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWords([]byte(tt.content))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("got %d words, want %d", len(got), tt.want)
			}
		})
	}
}

// TestNewRejectsContradictions asserts the constructor fails closed on
// dictionaries that cannot be interpreted or contradict each other.
func TestNewRejectsContradictions(t *testing.T) {
	tests := []struct {
		name          string
		misspellings  map[string]string
		allowed       map[string]bool
		wantErrSubstr []string
	}{
		{
			name:          "word both allowed and misspelled",
			misspellings:  map[string]string{"strkey": "key string"},
			allowed:       map[string]bool{"strkey": true},
			wantErrSubstr: []string{"both a project word and a misspelling"},
		},
		{
			name:          "allowed word with digits",
			allowed:       map[string]bool{"sha2": true},
			wantErrSubstr: []string{"not a lowercase alphabetic token"},
		},
		{
			name:          "empty allowed word",
			allowed:       map[string]bool{"": true},
			wantErrSubstr: []string{"not a lowercase alphabetic token"},
		},
		{
			name:          "empty correction",
			misspellings:  map[string]string{"recieve": " "},
			wantErrSubstr: []string{"has an empty correction"},
		},
		{
			name:          "multiple problems are all reported",
			misspellings:  map[string]string{"recieve": "", "SHA2": "sha"},
			wantErrSubstr: []string{"has an empty correction", "not a lowercase alphabetic token"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.misspellings, tt.allowed)
			if err == nil {
				t.Fatal("New accepted a contradictory dictionary")
			}
			for _, substr := range tt.wantErrSubstr {
				if !strings.Contains(err.Error(), substr) {
					t.Errorf("error %q does not mention %q", err, substr)
				}
			}
		})
	}
}

// TestFindDocsAndExclusions covers corpus discovery: markdown suffixes in
// any directory are found, the recorded provenance files are excluded, and
// a .git directory is not descended into.
func TestFindDocsAndExclusions(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "x")
	write("docs/guide.markdown", "x")
	write("e2e/RESULTS.md", "x")
	write("CHANGELOG.md", "x")
	write("LICENSE", "x")
	write("docs/recorded/notes.md", "x")
	write(".git/HEAD.md", "x")
	write("not-a-doc.txt", "x")

	got, err := FindDocs(os.DirFS(root))
	if err != nil {
		t.Fatalf("FindDocs: %v", err)
	}
	want := []string{"README.md", "docs/guide.markdown"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}

	for _, rel := range []string{"CHANGELOG.md", "e2e/RESULTS.md", "LICENSE", "docs/recorded/notes.md"} {
		if !DocExcluded(rel) {
			t.Errorf("%q should be excluded", rel)
		}
	}
	if DocExcluded("docs/guide.markdown") {
		t.Error("docs/guide.markdown should be checked")
	}
}

// TestWalkDocsEndToEnd runs WalkDocs against a small on-disk repository and
// asserts findings come back ordered across files with real line numbers.
func TestWalkDocsEndToEnd(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(MisspellingsFile, "recieve->receive\n")
	write(WordsFile, "soroban\n")
	write("b.md", "# B\n\nwe recieve the entry\n")
	write("a.md", "line one\nrecieve it\n")
	write("CHANGELOG.md", "recieve\n") // excluded

	got, err := WalkDocs(root)
	if err != nil {
		t.Fatalf("WalkDocs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %s, want two findings", formatFindings(got))
	}
	if got[0].File != "a.md" || got[0].Line != 2 {
		t.Errorf("first finding = %+v, want a.md line 2", got[0])
	}
	if got[1].File != "b.md" || got[1].Line != 3 {
		t.Errorf("second finding = %+v, want b.md line 3", got[1])
	}
}

// TestWalkDocsRejectsNonUTF8 asserts a corpus file that is not valid UTF-8
// stops the walk instead of reporting positions against mis-decoded text.
func TestWalkDocsRejectsNonUTF8(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, content []byte) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(MisspellingsFile, []byte(""))
	write(WordsFile, []byte(""))
	write("bad.md", []byte{'f', 'i', 'n', 'e', '\xff', '\n'})

	_, err := WalkDocs(root)
	var nonUTF8 *NonUTF8Error
	if !errors.As(err, &nonUTF8) || nonUTF8.File != "bad.md" {
		t.Fatalf("error = %v, want a NonUTF8Error for bad.md", err)
	}
}

// TestCommittedDictionariesAreValid checks the two real files in
// testdata/spell parse and cross-check. It is the test that keeps a bad
// row from reaching CI.
func TestCommittedDictionariesAreValid(t *testing.T) {
	misspellData, err := os.ReadFile(filepath.Join("..", "..", MisspellingsFile))
	if err != nil {
		t.Fatalf("reading %s: %v", MisspellingsFile, err)
	}
	wordsData, err := os.ReadFile(filepath.Join("..", "..", WordsFile))
	if err != nil {
		t.Fatalf("reading %s: %v", WordsFile, err)
	}

	misspellings, err := ParseMisspellings(misspellData)
	if err != nil {
		t.Fatalf("ParseMisspellings: %v", err)
	}
	words, err := ParseWords(wordsData)
	if err != nil {
		t.Fatalf("ParseWords: %v", err)
	}
	if _, err := New(misspellings, words); err != nil {
		t.Fatalf("New on the committed dictionaries: %v", err)
	}
	if len(misspellings) < 50 {
		t.Errorf("committed misspellings list has only %d rows; the corpus cross-product should keep it well above this", len(misspellings))
	}
	if len(words) < 5 {
		t.Errorf("committed project-word list has only %d rows", len(words))
	}
}

// formatFindings renders findings for test failure messages.
func formatFindings(fs []Finding) string {
	if len(fs) == 0 {
		return "(no findings)"
	}
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "\n  %s:%d:%d %s [%s]", f.File, f.Line, f.Col, f.Word, f.Rule)
		if f.Fix != "" {
			fmt.Fprintf(&b, " -> %s", f.Fix)
		}
	}
	return b.String()
}
