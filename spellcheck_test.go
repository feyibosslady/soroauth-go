package soroauth

import (
	"os"
	"strings"
	"testing"

	"github.com/soroauth/soroauth-go/internal/spellcheck"
)

// TestDocumentationProseIsClean runs the documentation spell check
// (issue #151) as part of the ordinary test suite, so `go test ./...`
// gates the docs the same way CI does. The rules live in
// internal/spellcheck and the dictionaries in testdata/spell; see
// CONTRIBUTING.md, "Spell-checking the documentation".
//
// The check itself is a misspelling matcher plus a doubled-word rule, not
// a dictionary lookup, so it cannot fail on legitimate protocol vocabulary
// it has never seen; the .github/workflows/docs.yml job runs the same walk
// on pull requests that touch documentation.
func TestDocumentationProseIsClean(t *testing.T) {
	// Tests run from the package directory, which is the repository root
	// for the root package; WalkDocs takes the root explicitly so the same
	// helper serves the CLI and the docs workflow too.
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("finding the repository root: %v", err)
	}
	findings, err := spellcheck.WalkDocs(root)
	if err != nil {
		t.Fatalf("walking the documentation: %v", err)
	}
	for _, f := range findings {
		switch f.Rule {
		case spellcheck.RuleMisspelling:
			t.Errorf("%s:%d:%d: %q is a known misspelling of %q", f.File, f.Line, f.Col, f.Word, f.Fix)
		case spellcheck.RuleDoubled:
			t.Errorf("%s:%d:%d: %q repeats a word", f.File, f.Line, f.Col, f.Word)
		default:
			t.Errorf("%s:%d:%d: %q [%s]", f.File, f.Line, f.Col, f.Word, f.Rule)
		}
	}
}

// docsThatMustLinkTheSpellCheck names the documents issue #151 requires to
// make the spell check reachable, and the link text each one carries.
var docsThatMustLinkTheSpellCheck = []struct {
	file     string
	linkText string
}{
	{file: "README.md", linkText: "Spell-checking the documentation"},
	{file: "ARCHITECTURE.md", linkText: "Spell-checking the documentation"},
}

// TestDocumentationLinksTheSpellCheck is the coverage clause of issue #151:
// the spell check must be reachable from the two index documents. It
// asserts each named file links CONTRIBUTING.md's
// "Spell-checking the documentation" section, so a future edit that drops
// the links fails this test instead of quietly orphaning the checker. This
// is the same enforcement style as TestReadmeSnippetsMatchTheirSource: the
// docs are gated by tests that can fail.
func TestDocumentationLinksTheSpellCheck(t *testing.T) {
	for _, doc := range docsThatMustLinkTheSpellCheck {
		t.Run(doc.file, func(t *testing.T) {
			raw, err := os.ReadFile(doc.file)
			if err != nil {
				t.Fatalf("reading %s: %v", doc.file, err)
			}
			content := string(raw)
			// The anchor GitHub generates for "Spell-checking the
			// documentation" is spell-checking-the-documentation, reached via
			// a CONTRIBUTING.md link.
			anchor := "contributing.md#spell-checking-the-documentation"
			if !strings.Contains(content, doc.linkText) {
				t.Errorf("%s does not mention %q; it must link the spell-check section of CONTRIBUTING.md (issue #151)", doc.file, doc.linkText)
			}
			if !strings.Contains(strings.ToLower(content), anchor) {
				t.Errorf("%s does not link CONTRIBUTING.md's %q section (expected the %q anchor); it must link the spell-check section (issue #151)", doc.file, doc.linkText, anchor)
			}
		})
	}
}
