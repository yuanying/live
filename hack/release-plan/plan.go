package main

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The author names git records for the bots whose commits are released
// without a person: Renovate's dependency updates, and the fixes of
// vulnerabilities the vuln-responder agent of fraction-agents opens through
// its GitHub App (348762456+fraction-vuln-responder[bot]@users.noreply.github.com).
const (
	RenovateAuthor      = "renovate[bot]"
	VulnResponderAuthor = "fraction-vuln-responder[bot]"
)

var botAuthors = []string{RenovateAuthor, VulnResponderAuthor}

// Trigger is what started the run deciding the release.
type Trigger string

const (
	// Weekly is the scheduled run, or a dispatch of it.
	Weekly Trigger = "weekly"
	// VulnFix is the merge of a pull request from a vuln-responder/ branch.
	VulnFix Trigger = "vuln-fix"
)

// Version is a release version, vMAJOR.MINOR.PATCH.
type Version struct {
	Major, Minor, Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) less(o Version) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

var versionTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// LatestVersion picks the highest vX.Y.Z among tags, compared as numbers.
// Anything else — a fourth part, a pre-release, a missing v — is not a
// version this repository releases and is skipped.
func LatestVersion(tags []string) (Version, bool) {
	var latest Version
	found := false
	for _, tag := range tags {
		m := versionTag.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		var v Version
		var err error
		if v.Major, err = strconv.Atoi(m[1]); err != nil {
			continue
		}
		if v.Minor, err = strconv.Atoi(m[2]); err != nil {
			continue
		}
		if v.Patch, err = strconv.Atoi(m[3]); err != nil {
			continue
		}
		if !found || latest.less(v) {
			latest, found = v, true
		}
	}
	return latest, found
}

// imageContent is what the Dockerfile builds the image from: the Dockerfile
// itself, the module files and the Go files of the package at the root, which
// is the one `go build .` compiles. hack/ is copied into the builder too, but
// nothing of it reaches the image.
var imageContent = []string{"Dockerfile", "go.mod", "go.sum", "*.go"}

// IsImageContent reports whether a change to file can change the image. The
// patterns are matched as path.Match does, so `*` stays within the root.
func IsImageContent(file string) bool {
	for _, p := range imageContent {
		if ok, _ := path.Match(p, file); ok {
			return true
		}
	}
	return false
}

// Commit is one non-merge commit between the latest version and HEAD.
type Commit struct {
	Hash    string
	Author  string
	Subject string
}

// Plan is what the run does. Without a release, Version and Message are empty
// and Reason says why nothing is released.
type Plan struct {
	Release bool
	Version string
	// Message is the annotated tag's message.
	Message string
	Reason  string
	// Next is the next patch version, set whenever there are commits to
	// release, so that a run that holds back can say what a person would tag.
	Next string
	// Held are the commits by neither bot, which leave the release to a person.
	Held []Commit
}

// Decide whether the commits since latest are released as the next patch
// version. It is released only when every commit is Renovate's or the
// vulnerability responder's and at least one changed path is part of the
// image. Anything a person wrote is released by that person, who chooses the
// version and writes the message.
func Decide(latest Version, commits []Commit, changed []string, trigger Trigger) Plan {
	if len(commits) == 0 {
		return Plan{Reason: fmt.Sprintf("no commits since %s", latest)}
	}
	next := Version{Major: latest.Major, Minor: latest.Minor, Patch: latest.Patch + 1}
	bots := strings.Join(botAuthors, " or ")

	var held []Commit
	for _, c := range commits {
		if !slices.Contains(botAuthors, c.Author) {
			held = append(held, c)
		}
	}
	if len(held) > 0 {
		c := held[0]
		return Plan{
			Reason: fmt.Sprintf("%q by %s is not by %s; releasing it is left to a person", c.Subject, c.Author, bots),
			Next:   next.String(),
			Held:   held,
		}
	}
	if !slices.ContainsFunc(changed, IsImageContent) {
		return Plan{
			Reason: fmt.Sprintf("the bots' commits since %s change nothing the image is built from (%s)", latest, strings.Join(imageContent, ", ")),
			Next:   next.String(),
		}
	}

	by := "the weekly workflow"
	if trigger == VulnFix {
		by = "the workflow on the merge of a vulnerability fix"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: dependency updates\n\n", next)
	fmt.Fprintf(&b, "Only the dependency updates of Renovate and of the vulnerability\nresponder since %s, released by %s:\n\n", latest, by)
	for _, c := range commits {
		fmt.Fprintf(&b, "- %s\n", c.Subject)
	}
	return Plan{
		Release: true,
		Version: next.String(),
		Message: b.String(),
		Reason:  fmt.Sprintf("%d commits since %s, all by %s, and the image's content changed", len(commits), latest, bots),
		Next:    next.String(),
	}
}

// Summary is the Markdown the run of a vulnerability fix leaves in its job
// summary when it holds the release back: why, the commits that held it, and
// the commands a person runs to release sha themselves. A pushed tag starts
// image.yaml, which publishes the image. A release is reported by the job that
// tags it, so its summary is empty.
func Summary(plan Plan, sha string) string {
	if plan.Release {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### No release\n\n%s\n", plan.Reason)
	if len(plan.Held) > 0 {
		b.WriteString("\nCommits that held the release back:\n\n")
		for _, c := range plan.Held {
			fmt.Fprintf(&b, "- `%.12s` %s: %s\n", c.Hash, c.Author, c.Subject)
		}
	}
	if plan.Next != "" {
		fmt.Fprintf(&b, "\nTo release it by hand, with a message of your own:\n\n```sh\ngit fetch origin\ngit tag -a %s %s\ngit push origin %s\n```\n", plan.Next, sha, plan.Next)
	}
	return b.String()
}
