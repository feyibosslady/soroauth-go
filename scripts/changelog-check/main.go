package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var issueRE = regexp.MustCompile(`#(\d+)`)

type commit struct {
	hash   string
	title  string
	body   string
	issues []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: changelog-check <changelog.md>")
		return 2
	}

	baseRef, err := comparisonBase()
	if err != nil {
		fmt.Fprintf(stderr, "finding comparison base: %v\n", err)
		return 1
	}
	base, err := mergeBase("HEAD", baseRef)
	if err != nil {
		fmt.Fprintf(stderr, "finding merge base with %q: %v\n", baseRef, err)
		return 1
	}

	log, err := gitLog(base + "..HEAD")
	if err != nil {
		fmt.Fprintf(stderr, "reading commits: %v\n", err)
		return 1
	}
	parsedCommits, err := parseGitLog(log)
	if err != nil {
		fmt.Fprintf(stderr, "parsing commit log: %v\n", err)
		return 1
	}
	commits := filterFeatFix(parsedCommits)

	changelogIssues, err := parseChangelogIssues(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "reading Unreleased changelog entries: %v\n", err)
		return 1
	}
	prIssues, err := pullRequestIssues()
	if err != nil {
		fmt.Fprintf(stderr, "reading pull request issue references: %v\n", err)
		return 1
	}

	matched, missing := 0, make([]commit, 0)
	for _, c := range commits {
		issues := c.issues
		if len(issues) == 0 {
			issues = prIssues
		}
		if intersects(issues, changelogIssues) {
			matched++
		} else {
			missing = append(missing, c)
		}
	}

	fmt.Fprintf(stdout, "Compared commits since %s\n", base)
	fmt.Fprintf(stdout, "%d feat/fix commits checked\n", len(commits))
	fmt.Fprintf(stdout, "%d matched an Unreleased changelog issue\n", matched)
	fmt.Fprintf(stdout, "%d missing a matching Unreleased entry\n", len(missing))
	if len(missing) == 0 {
		fmt.Fprintln(stdout, "CHANGELOG consistency check passed")
		return 0
	}

	for _, c := range missing {
		fmt.Fprintf(stderr, "  %s %s: add an issue reference that appears in CHANGELOG.md\n", shortHash(c.hash), c.title)
	}
	return 1
}

func comparisonBase() (string, error) {
	if ref := os.Getenv("CHANGELOG_BASE_REF"); ref != "" {
		return ref, nil
	}
	if ref := os.Getenv("GITHUB_BASE_REF"); ref != "" {
		return "origin/" + ref, nil
	}
	if before := os.Getenv("GITHUB_EVENT_BEFORE"); before != "" {
		if strings.Trim(before, "0") == "" {
			return "", errors.New("push event has no prior commit to compare")
		}
		return before, nil
	}
	for _, ref := range []string{"origin/main", "upstream/main"} {
		if command := exec.Command("git", "rev-parse", "--verify", "--quiet", ref+"^{commit}"); command.Run() == nil {
			return ref, nil
		}
	}
	return "", fmt.Errorf("no PR base, push base, or main reference found; set CHANGELOG_BASE_REF")
}

func mergeBase(left, right string) (string, error) {
	output, err := exec.Command("git", "merge-base", left, right).Output()
	if err != nil {
		return "", fmt.Errorf("git merge-base %s %s: %w", left, right, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func gitLog(revision string) (string, error) {
	command := exec.Command("git", "log", "--format=%H%x1f%s%x1f%b%x1e", revision)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git log %s: %w", revision, err)
	}
	return string(output), nil
}

func parseGitLog(output string) ([]commit, error) {
	var commits []commit
	for _, record := range strings.Split(output, "\x1e") {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x1f", 3)
		if len(fields) != 3 || fields[0] == "" || fields[1] == "" {
			return nil, fmt.Errorf("malformed git log record %q", record)
		}
		commits = append(commits, commit{
			hash:   fields[0],
			title:  fields[1],
			body:   fields[2],
			issues: extractIssues(fields[1] + "\n" + fields[2]),
		})
	}
	return commits, nil
}

func filterFeatFix(commits []commit) []commit {
	filtered := make([]commit, 0, len(commits))
	for _, c := range commits {
		title := strings.ToLower(c.title)
		if strings.HasPrefix(title, "feat") || strings.HasPrefix(title, "fix") {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func shortHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func parseChangelogIssues(path string) (map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	issues := make(map[string]bool)
	inUnreleased := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			inUnreleased = line == "## [Unreleased]" || line == "## Unreleased"
			continue
		}
		if inUnreleased {
			for _, issue := range extractIssues(line) {
				issues[issue] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return issues, nil
}

func pullRequestIssues() ([]string, error) {
	path := os.Getenv("GITHUB_EVENT_PATH")
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePullRequestIssues(data)
}

func parsePullRequestIssues(data []byte) ([]string, error) {
	var event struct {
		PullRequest *struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	if event.PullRequest == nil {
		return nil, nil
	}
	return extractIssues(event.PullRequest.Title + "\n" + event.PullRequest.Body), nil
}

func extractIssues(text string) []string {
	matches := issueRE.FindAllStringSubmatch(text, -1)
	issues := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		if !seen[match[1]] {
			issues = append(issues, match[1])
			seen[match[1]] = true
		}
	}
	return issues
}

func intersects(issues []string, changelogIssues map[string]bool) bool {
	for _, issue := range issues {
		if changelogIssues[issue] {
			return true
		}
	}
	return false
}
