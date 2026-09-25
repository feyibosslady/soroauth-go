// Package spellcheck checks Markdown prose against committed dictionaries of
// known misspellings and project terms.
//
// It is a codespell-style misspelling matcher, deliberately not a dictionary
// lookup: a token is flagged because it is on the committed misspelling list
// (testdata/spell/misspellings.txt), or because an English function word
// repeats ("the the"), never because it is an unknown word. That keeps the
// check deterministic, offline and free of a vendored English word list, and
// it is why protocol terms (soroban, strkey, XDR, ed25519, CAP-71-01) can
// never fail it: an unlisted word is simply not a finding. The cost is
// stated rather than hidden — a novel misspelling is caught only once
// someone adds it to the list. See CONTRIBUTING.md, "Spell-checking the
// documentation".
//
// Prose extraction skips fenced code blocks, inline code spans, link and
// image targets, bare URLs, reference-link labels and HTML comments: code,
// identifiers and URLs are not English prose. Everything else is tokenized
// (runs of letters, split at lower→upper boundaries, case-folded) and
// checked.
package spellcheck

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The two rule names a Finding can carry.
const (
	RuleMisspelling = "misspelling"
	RuleDoubled     = "doubled"
)

// Finding is one prose problem, positioned in the original file. Line and
// Col are 1-based; Col counts runes, so it matches what an editor shows on
// UTF-8 content. Fix is the committed correction for a misspelling and is
// empty for a doubled word.
type Finding struct {
	File string
	Line int
	Col  int
	Word string // the text as written, e.g. "Recieve" or "the the"
	Rule string // RuleMisspelling or RuleDoubled
	Fix  string // suggested correction; empty for RuleDoubled
}

// Checker reports known misspellings and doubled words in Markdown prose.
// The zero value is not usable; build one with New.
type Checker struct {
	misspellings map[string]string
	allowed      map[string]bool
}

// New builds a Checker and fails closed on a contradictory or malformed
// dictionary: an empty or non-alphabetic entry, a correction identical to
// its typo, duplicate entries with different corrections, or a word that is
// both a project term and a misspelling (the caller must pick one). A
// dictionary that cannot be interpreted must stop the run here rather than
// silently never match.
func New(misspellings map[string]string, allowed map[string]bool) (*Checker, error) {
	var errs []error

	allowedSet := make(map[string]bool, len(allowed))
	for w := range allowed {
		if !isDictionaryToken(w) {
			errs = append(errs, fmt.Errorf("project word %q is not a lowercase alphabetic token", w))
			continue
		}
		allowedSet[w] = true
	}

	for typo, fix := range misspellings {
		if !isDictionaryToken(typo) {
			errs = append(errs, fmt.Errorf("misspelling entry %q is not a lowercase alphabetic token", typo))
			continue
		}
		if strings.TrimSpace(fix) == "" {
			errs = append(errs, fmt.Errorf("misspelling entry %q has an empty correction", typo))
			continue
		}
		if typo == strings.ToLower(strings.TrimSpace(fix)) {
			errs = append(errs, fmt.Errorf("misspelling entry %q corrects to itself", typo))
			continue
		}
		if allowedSet[typo] {
			errs = append(errs, fmt.Errorf("%q is both a project word and a misspelling; remove one of the two rows", typo))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &Checker{misspellings: misspellings, allowed: allowedSet}, nil
}

// isDictionaryToken is the format contract for both dictionaries: a
// non-empty run of lowercase ASCII letters. Tokenization is case-folded
// letter runs, so an entry the checker could never see (one with digits,
// underscores or uppercase) would be dead weight pretending to be
// protection, and is rejected instead.
func isDictionaryToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// ParseMisspellings parses the committed misspellings file: one
// "typo->correction" pair per line, blank lines and #-comments ignored.
// A duplicate typo with a different correction is an error, because the
// two rows contradict each other and whichever the map kept would be
// arbitrary.
func ParseMisspellings(data []byte) (map[string]string, error) {
	out := make(map[string]string)
	var errs []error
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		typo, fix, ok := strings.Cut(line, "->")
		if !ok {
			errs = append(errs, fmt.Errorf("misspellings line %d: %q has no -> separator", i+1, line))
			continue
		}
		typo = strings.ToLower(strings.TrimSpace(typo))
		fix = strings.TrimSpace(fix)
		if !isDictionaryToken(typo) {
			errs = append(errs, fmt.Errorf("misspellings line %d: %q is not a lowercase alphabetic token", i+1, line))
			continue
		}
		if fix == "" {
			errs = append(errs, fmt.Errorf("misspellings line %d: %q has an empty correction", i+1, line))
			continue
		}
		if typo == strings.ToLower(fix) {
			errs = append(errs, fmt.Errorf("misspellings line %d: %q corrects to itself", i+1, typo))
			continue
		}
		if prev, dup := out[typo]; dup && prev != fix {
			errs = append(errs, fmt.Errorf("misspellings line %d: %q is listed twice with different corrections", i+1, typo))
			continue
		}
		out[typo] = fix
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

// ParseWords parses the committed project-words file: one lowercase token
// per line, blank lines and #-comments ignored. A duplicate line is an
// error so the file cannot grow two rows that a later edit only half-updates.
func ParseWords(data []byte) (map[string]bool, error) {
	out := make(map[string]bool)
	var errs []error
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if out[line] {
			errs = append(errs, fmt.Errorf("words line %d: %q is listed twice", i+1, line))
			continue
		}
		if !isDictionaryToken(strings.ToLower(line)) {
			errs = append(errs, fmt.Errorf("words line %d: %q is not a lowercase alphabetic token", i+1, line))
			continue
		}
		out[strings.ToLower(line)] = true
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

// doubledWords are English function words whose immediate repetition in
// prose is essentially always a typo. Deliberately conservative: words
// where a repeat can be grammatical ("that that", "had had", "very very")
// are excluded, because a false positive trains people to ignore the check.
// This is an English rule, not project vocabulary, so it lives in code;
// the project vocabulary that must never be flagged lives in
// testdata/spell/words.txt.
var doubledWords = map[string]bool{
	"about": true, "after": true, "against": true, "all": true, "also": true,
	"an": true, "and": true, "any": true, "are": true, "as": true,
	"at": true, "be": true, "been": true, "before": true, "because": true,
	"between": true, "both": true, "but": true, "by": true, "can": true,
	"could": true, "did": true, "do": true, "does": true, "down": true,
	"during": true, "each": true, "every": true, "first": true, "for": true,
	"from": true, "has": true, "have": true, "how": true, "if": true,
	"in": true, "into": true, "is": true, "it": true, "its": true,
	"last": true, "less": true, "may": true, "might": true, "more": true,
	"most": true, "must": true, "no": true, "nor": true, "not": true,
	"of": true, "on": true, "only": true, "onto": true, "or": true,
	"other": true, "our": true, "out": true, "over": true, "own": true,
	"per": true, "same": true, "shall": true, "should": true, "so": true,
	"some": true, "such": true, "than": true, "the": true, "their": true,
	"them": true, "then": true, "there": true, "these": true, "they": true,
	"this": true, "those": true, "through": true, "to": true, "too": true,
	"two": true, "under": true, "until": true, "up": true, "us": true,
	"via": true, "was": true, "were": true, "what": true, "when": true,
	"where": true, "which": true, "while": true, "who": true, "will": true,
	"without": true, "within": true, "would": true, "you": true, "your": true,
}

// Check runs the checker over one Markdown document and returns its
// findings in file order. It never mutates content and never fails: a
// document with no findings produces an empty result.
func (c *Checker) Check(file, content string) []Finding {
	// Multi-line constructs are blanked byte-for-byte before line splitting,
	// so every byte offset in the blanked text maps to the same offset in
	// the original and reported positions stay exact.
	prose := string(blankHTMLComments([]byte(content)))
	lines := strings.SplitAfter(prose, "\n")

	var findings []Finding
	var fence fenceState
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// The fence boundary is decided on the comment-blanked line; inline
		// blanking is skipped for fence lines and fence bodies, which carry
		// no prose.
		if fenceTransition(trimmed, &fence) {
			continue
		}
		if fence.open {
			continue
		}
		findings = append(findings, c.checkLine(file, i+1, line, blankInlineMarkdown(line))...)
	}
	return findings
}

// fenceState tracks whether a fenced code block is open, and the marker
// character and minimum length that will close it.
type fenceState struct {
	open   bool
	marker byte
	length int
}

// fenceTransition reports whether the line opens or closes a fenced code
// block, updating st when it does. A fence is a line whose trimmed content
// starts with a run of at least three backticks or tildes; a closing fence
// must use the same character as the opening one and be at least as long,
// so a ~~~ line inside a ``` block is content, not a close. Fence info
// strings (```go) need no special handling: only the leading marker run
// matters.
func fenceTransition(trimmed string, st *fenceState) bool {
	if trimmed == "" {
		return false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return false
	}
	if !st.open {
		st.open, st.marker, st.length = true, c, n
		return true
	}
	if c == st.marker && n >= st.length {
		st.open, st.marker, st.length = false, 0, 0
	}
	// Either the fence closed, or this is a lookalike line inside the fence
	// (content). Both leave the state consistent and the line skipped.
	return true
}

// checkLine checks one line's prose. original supplies the text positions
// are counted against; prose is the same line with non-prose regions
// blanked to spaces (byte offsets identical between the two).
func (c *Checker) checkLine(file string, lineNo int, original string, prose []byte) []Finding {
	var findings []Finding
	var prevLower, prevWord string
	var prevEnd int
	havePrev := false

	for _, run := range letterRuns(prose) {
		for _, st := range camelSplit(run) {
			lower := strings.ToLower(st.text)
			if len(lower) < 2 {
				// Single letters ("a", "I", the "M" in "M-of-N") carry no
				// spelling information and do not participate in pairs.
				havePrev = false
				continue
			}

			if havePrev && lower == prevLower && doubledWords[lower] &&
				!c.allowed[lower] && isBlankGap(prose[prevEnd:st.start]) {
				findings = append(findings, Finding{
					File: file,
					Line: lineNo,
					Col:  runeCol(original, st.start),
					Word: prevWord + " " + st.text,
					Rule: RuleDoubled,
				})
				prevLower, prevWord, prevEnd, havePrev = lower, st.text, st.end, true
				continue
			}

			if !c.allowed[lower] {
				if fix, misspelled := c.misspellings[lower]; misspelled {
					findings = append(findings, Finding{
						File: file,
						Line: lineNo,
						Col:  runeCol(original, st.start),
						Word: st.text,
						Rule: RuleMisspelling,
						Fix:  fix,
					})
				} else if base, suffix, fix, matched := c.matchInflection(lower); matched {
					findings = append(findings, Finding{
						File: file,
						Line: lineNo,
						Col:  runeCol(original, st.start),
						Word: st.text,
						Rule: RuleMisspelling,
						Fix:  inflect(fix, base, suffix),
					})
				}
			}
			prevLower, prevWord, prevEnd, havePrev = lower, st.text, st.end, true
		}
	}
	return findings
}

// inflections are the suffixes the checker strips when a token is not
// itself listed: English prose inflects, and a dictionary that only caught
// the citation form would wave "recieves" through while flagging
// "recieve". The set is deliberately closed and mechanical — each suffix
// has one strip rule and one re-attach rule (inflect) — so a match is
// reproducible, not a guess. Suffixes are ordered longest-first by the
// match loop; a token matches at most one entry. Words whose listed typo
// already ends in one of these suffixes are unaffected: the exact form is
// always tried before any stripping.
var inflections = []struct {
	suffix   string // the token's suffix
	suffixOf string // the suffix it strips from the base form ("" = none)
}{
	{suffix: "ies", suffixOf: "y"}, // carry->carries style: base y->ies
	{suffix: "ied", suffixOf: "y"}, // carried
	{suffix: "ies" + "", suffixOf: "y"},
	{suffix: "ing", suffixOf: ""},  // receiving (base may drop a trailing e)
	{suffix: "ings", suffixOf: ""}, // writings
	{suffix: "ed", suffixOf: ""},   // received
	{suffix: "es", suffixOf: ""},   // bunches; also plain -s words ending in s/x/z/ch/sh
	{suffix: "s", suffixOf: ""},    // receives
	{suffix: "ly", suffixOf: ""},   // definitely->definately-ly style adverbs
}

// matchInflection looks a token up in the misspellings list after stripping
// one mechanical inflection. It returns the stripped base (a listed typo),
// the matched suffix rule, the base's correction, and whether anything
// matched. The exact form is always tried by the caller first, so a listed
// typo that happens to end in a suffix-looking run (e.g. "cross" ending in
// "ss" never matches here; "cactus" ending in "us" is not a rule) is found
// exactly or not at all. Only rules whose stripped result is a listed typo
// produce a match: stripping cannot invent a finding from an ordinary word
// like "crosses" (strip "es" -> "cross", unlisted, no match) — it can only
// extend a row that already exists.
func (c *Checker) matchInflection(token string) (base, suffix, fix string, matched bool) {
	for _, infl := range inflections {
		if len(token) <= len(infl.suffix)+2 || !strings.HasSuffix(token, infl.suffix) {
			continue
		}
		stem := token[:len(token)-len(infl.suffix)]
		candidates := []string{stem}
		if infl.suffixOf != "" {
			candidates = append(candidates, stem+infl.suffixOf)
		}
		// -ing/-ed also drop a doubled final consonant and a trailing e:
		// "occured"+"ed" -> "occur"/"occure"; the listed typo is "occured".
		if infl.suffix == "ing" || infl.suffix == "ed" || infl.suffix == "ings" {
			if n := len(stem); n >= 2 && stem[n-1] == stem[n-2] && !isVowel(stem[n-1]) {
				candidates = append(candidates, stem[:n-1])
			}
			candidates = append(candidates, stem+"e")
		}
		for _, cand := range candidates {
			if cand == token {
				continue
			}
			if baseFix, listed := c.misspellings[cand]; listed {
				return cand, infl.suffix, baseFix, true
			}
		}
	}
	return "", "", "", false
}

// inflect re-attaches the matched suffix to a correction, applying the
// same mechanical rules in reverse so "recieves" suggests "receives" and
// not "receivees". Corrections already ending in the suffix-adjacent shape
// are returned as-is when re-attachment would double a letter that the
// rule strip removed (e.g. base "commited" corrects to "committed"; the
// token "commitedly" never occurs, but the arithmetic stays honest).
func inflect(correction, base, suffix string) string {
	// The strip rule that matched removed `suffix` from the typo. The
	// correction is the right spelling of the base; re-attach the same
	// suffix, with the same candidate logic in reverse, preferring the
	// plain attachment.
	plain := correction + suffix
	// e-drop reversal: receive+ing = receiving, not receiveing.
	if suffix == "ing" || suffix == "ings" {
		if strings.HasSuffix(correction, "e") {
			return correction[:len(correction)-1] + suffix
		}
	}
	// y->ies / y->ied reversal: carry+ies = carries.
	if suffix == "ies" || suffix == "ied" {
		if strings.HasSuffix(correction, "y") {
			return correction[:len(correction)-1] + suffix
		}
	}
	_ = base
	return plain
}

// isVowel reports whether b is a lowercase ASCII vowel.
func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// isBlankGap reports whether the text between two tokens is at least one
// byte of whitespace and nothing else. "the, the" and "the-the" are not
// doubled-word typos, and neither is the glue in "theThe".
func isBlankGap(s []byte) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

// runeCol converts a byte offset within line into a 1-based rune column,
// counting against the original (unblanked) line so multibyte characters
// before the finding do not shift the column.
func runeCol(line string, offset int) int {
	if offset > len(line) {
		offset = len(line)
	}
	return utf8.RuneCountInString(line[:offset]) + 1
}

// letterRuns returns the maximal runs of letters in s. Everything else —
// digits, punctuation, apostrophes, hyphens — separates runs, which is what
// makes "CAP-71-01" check as "cap", "don't" as "don" and "soroauth's" as
// "soroauth" without special cases.
func letterRuns(s []byte) []span {
	var runs []span
	start := -1
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRune(s[i:])
		if r < utf8.RuneSelf && asciiLetter(s[i]) {
			if start < 0 {
				start = i
			}
			i += size
			continue
		}
		if start >= 0 {
			runs = append(runs, span{text: string(s[start:i]), start: start, end: i})
			start = -1
		}
		i += size
	}
	if start >= 0 {
		runs = append(runs, span{text: string(s[start:]), start: start, end: len(s)})
	}
	return runs
}

// asciiLetter reports whether b is an ASCII letter. Non-ASCII letters are
// deliberately not prose tokens: the corpus is English, and a multibyte
// accented word inside prose is either a quoted name or an encoding
// accident, not an English misspelling to look up.
func asciiLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// camelSplit splits a letters-only run at lower→upper boundaries, so a typo
// glued into an identifier-shaped word is still seen: "recieveSomething"
// checks as "recieve" and "Something". Acronym runs ("XDR", "XDRPayload"
// without a lower/upper boundary) stay whole, which is correct — they are
// identifiers, not prose. Offsets stay in the coordinate system of the
// containing line, because the caller's column arithmetic depends on it.
func camelSplit(run span) []span {
	rs := []rune(run.text)
	if len(rs) < 2 {
		return []span{run}
	}
	var out []span
	start := 0
	byteOff := 0
	for i := 1; i < len(rs); i++ {
		if unicode.IsLower(rs[i-1]) && unicode.IsUpper(rs[i]) {
			text := string(rs[start:i])
			out = append(out, span{
				text:  text,
				start: run.start + byteOff,
				end:   run.start + byteOff + len(text),
			})
			byteOff += len(text)
			start = i
		}
	}
	text := string(rs[start:])
	out = append(out, span{
		text:  text,
		start: run.start + byteOff,
		end:   run.start + byteOff + len(text),
	})
	return out
}

// span is a byte-offset range in a single line, with its text.
type span struct {
	text       string
	start, end int // byte offsets within the line
}

// blankHTMLComments returns content with every HTML comment replaced
// byte-for-byte by spaces, newlines preserved so line numbers do not move.
// An unclosed comment blanks to the end of the document. Multibyte
// sequences are left in place: comment delimiters are ASCII, and leaving
// non-ASCII bytes keeps every offset and rune count exact.
func blankHTMLComments(content []byte) []byte {
	out := content
	for {
		start := bytes.Index(out, []byte("<!--"))
		if start < 0 {
			return out
		}
		end := bytes.Index(out[start:], []byte("-->"))
		if end < 0 {
			blankRange(out, start, len(out))
			return out
		}
		blankRange(out, start, start+end+3)
		// The opening marker is now spaces, so the next Index call finds
		// the next comment rather than this one.
	}
}

// blankInlineMarkdown returns line with its non-prose regions replaced
// byte-for-byte by spaces: inline code spans, bare URLs and autolinks,
// link/image targets, reference-link labels, and inline HTML comments.
// Emphasis, footnote and strikethrough markers need no blanking — they are
// non-letters and letterRuns already ignores them. Multibyte characters are
// left in place so columns stay exact.
func blankInlineMarkdown(line string) []byte {
	out := []byte(line)

	// A reference definition "[label]: https://…" is not prose: its label
	// is looked up, never rendered. The URL after the colon is caught by
	// the bare-URL pass below. A definition is distinguished from a
	// footnote-style link by the colon directly after the closing bracket.
	if trimmed := bytes.TrimLeft(out, " \t"); len(trimmed) > 0 && trimmed[0] == '[' {
		if end := bytes.IndexByte(trimmed, ']'); end > 0 && end+1 < len(trimmed) && trimmed[end+1] == ':' {
			offset := len(out) - len(trimmed)
			blankRange(out, offset, offset+end+1)
		}
	}

	// Inline code spans first: their content may hold markdown syntax that
	// the rules below would otherwise misread. A span opened with a run of
	// n backticks closes at the next run of exactly n.
	for i := 0; i < len(out); {
		if out[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(out) && out[i+n] == '`' {
			n++
		}
		closeIdx := indexBacktickRun(out, i+n, n)
		if closeIdx < 0 {
			i += n
			continue
		}
		blankRange(out, i, closeIdx+n)
		i = closeIdx + n
	}

	// Bare URLs and the inside of <https://…> autolinks: from the scheme to
	// the first stop character. Stopping at "(" and ")" protects link
	// targets, and at "<" and ">" protects autolink delimiters.
	for i := 0; i < len(out); {
		if isURLStart(out, i) {
			j := i
			for j < len(out) && !isURLStop(out[j]) {
				j++
			}
			blankRange(out, i, j)
			i = j
			continue
		}
		i++
	}

	// Link and image targets "](", and reference-link labels "][label]".
	// The link text in "[text]" stays: it is prose.
	for i := 0; i < len(out); i++ {
		if out[i] != ']' || i+1 >= len(out) {
			continue
		}
		switch out[i+1] {
		case '(':
			depth := 1
			j := i + 2
			for ; j < len(out); j++ {
				switch out[j] {
				case '(':
					depth++
				case ')':
					depth--
					if depth == 0 {
						goto closed
					}
				}
			}
			// Unbalanced: treat the rest of the line as target.
			blankRange(out, i+2, len(out))
			i = len(out) - 1
			continue
		closed:
			blankRange(out, i+2, j)
			i = j - 1
		case '[':
			j := i + 2
			for ; j < len(out) && out[j] != ']'; j++ {
			}
			if j < len(out) {
				blankRange(out, i+1, j+1)
				i = j
			}
		}
	}

	// Inline HTML comments.
	for i := 0; i+4 <= len(out); {
		if !bytes.Equal(out[i:i+4], []byte("<!--")) {
			i++
			continue
		}
		end := bytes.Index(out[i:], []byte("-->"))
		if end < 0 {
			blankRange(out, i, len(out))
			break
		}
		blankRange(out, i, i+end+3)
		i += end + 3
	}

	return out
}

// indexBacktickRun returns the offset of the first run of exactly n
// backticks at or after from, or -1. A longer run does not close a span
// opened with n backticks, matching how markdown itself pairs them.
func indexBacktickRun(out []byte, from, n int) int {
	for i := from; i+n <= len(out); {
		if out[i] != '`' {
			i++
			continue
		}
		k := 0
		for i+k < len(out) && out[i+k] == '`' {
			k++
		}
		if k == n {
			return i
		}
		i += k
	}
	return -1
}

// isURLStart reports whether a bare URL begins at out[i].
func isURLStart(out []byte, i int) bool {
	if i+8 <= len(out) && string(out[i:i+8]) == "https://" {
		return true
	}
	return i+7 <= len(out) && string(out[i:i+7]) == "http://"
}

// isURLStop reports whether b ends a bare URL.
func isURLStop(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '(', ')', '<', '>', '"', '\'', '`':
		return true
	}
	return false
}

// blankRange replaces out[from:to] with spaces, preserving newlines (so
// line splitting still works) and multibyte sequences (so rune columns
// stay exact). Only ASCII non-newline bytes are blanked.
func blankRange(out []byte, from, to int) {
	for i := from; i < to && i < len(out); i++ {
		if out[i] >= utf8.RuneSelf || out[i] == '\n' {
			continue
		}
		out[i] = ' '
	}
}
