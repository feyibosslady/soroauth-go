package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseGitLogKeepsCommitBodyTogether(t *testing.T) {
	output := strings.Join([]string{
		"abc123\x1ffeat: add feature\x1fBody mentions #42.\n\nA second paragraph.",
		"def456\x1ffix: repair feature\x1fFixes #43.",
		"",
	}, "\x1e")

	commits, err := parseGitLog(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("parsed %d commits, want 2", len(commits))
	}
	if commits[0].title != "feat: add feature" || !strings.Contains(commits[0].body, "A second paragraph.") {
		t.Fatalf("first commit was split or truncated: %+v", commits[0])
	}
	if !reflect.DeepEqual(commits[0].issues, []string{"42"}) {
		t.Errorf("first commit issues = %v, want [42]", commits[0].issues)
	}
}

func TestParseChangelogIssuesOnlyUsesUnreleasedSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	content := `# Changelog

## [Unreleased]

### Added
- New behavior (#42)

## [1.0.0]
- Previous behavior (#41)
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	issues, err := parseChangelogIssues(path)
	if err != nil {
		t.Fatal(err)
	}
	if !issues["42"] || issues["41"] {
		t.Fatalf("parsed issue set = %v, want only #42", issues)
	}
}

func TestParsePullRequestIssues(t *testing.T) {
	issues, err := parsePullRequestIssues([]byte(`{
		"pull_request": {
			"title": "Implement feature",
			"body": "Closes #400\nCloses #401"
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(issues, []string{"400", "401"}) {
		t.Fatalf("pull request issues = %v, want [400 401]", issues)
	}
}
