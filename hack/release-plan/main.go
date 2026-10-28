// release-plan decides whether a run of .github/workflows/release.yaml (the
// weekly one, or the one on the merge of a vulnerability fix) releases the
// next patch version, reading the history of the checkout it runs in;
// `go run ./hack/release-plan` asks the same question on a laptop. The
// decision itself is Decide; this file only asks git and writes the result.
//
// It always prints the decision and its reason. With -github-output it also
// appends version and message to that file in the format of $GITHUB_OUTPUT,
// both empty when nothing is released, and sha, the commit decided on. With
// -step-summary it appends the Summary of a held release to that file.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	output := flag.String("github-output", "", "append version, message and sha to this file, in the format of $GITHUB_OUTPUT")
	summary := flag.String("step-summary", "", "append why nothing is released, and how to release it by hand, to this file")
	trigger := flag.String("trigger", string(Weekly), "what started the run: weekly or vuln-fix")
	flag.Parse()
	if t := Trigger(*trigger); t != Weekly && t != VulnFix {
		fmt.Fprintf(os.Stderr, "release-plan: -trigger %q is neither %s nor %s\n", *trigger, Weekly, VulnFix)
		os.Exit(2)
	}

	ctx := context.Background()
	plan, err := planFromGit(ctx, Trigger(*trigger))
	if err != nil {
		fmt.Fprintln(os.Stderr, "release-plan:", err)
		os.Exit(1)
	}

	if plan.Release {
		fmt.Printf("release %s: %s\n\n%s", plan.Version, plan.Reason, plan.Message)
	} else {
		fmt.Printf("no release: %s\n", plan.Reason)
	}

	if *output == "" && *summary == "" {
		return
	}
	head, err := git(ctx, "rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintln(os.Stderr, "release-plan:", err)
		os.Exit(1)
	}
	sha := strings.TrimSpace(head)
	if *output != "" {
		if err := writeOutput(*output, plan, sha); err != nil {
			fmt.Fprintln(os.Stderr, "release-plan:", err)
			os.Exit(1)
		}
	}
	if *summary != "" {
		if err := appendFile(*summary, Summary(plan, sha)); err != nil {
			fmt.Fprintln(os.Stderr, "release-plan:", err)
			os.Exit(1)
		}
	}
}

func planFromGit(ctx context.Context, trigger Trigger) (Plan, error) {
	tags, err := git(ctx, "tag", "--list")
	if err != nil {
		return Plan{}, err
	}
	latest, ok := LatestVersion(lines(tags))
	if !ok {
		return Plan{Reason: "no vX.Y.Z tag; the first version is a person's to choose"}, nil
	}

	rng := latest.String() + "..HEAD"
	log, err := git(ctx, "log", "--no-merges", "--reverse", "--format=%H%x1f%an%x1f%s", rng)
	if err != nil {
		return Plan{}, err
	}
	var commits []Commit
	for _, l := range lines(log) {
		hash, rest, _ := strings.Cut(l, "\x1f")
		author, subject, _ := strings.Cut(rest, "\x1f")
		commits = append(commits, Commit{Hash: hash, Author: author, Subject: subject})
	}

	changed, err := git(ctx, "diff", "--name-only", rng)
	if err != nil {
		return Plan{}, err
	}
	return Decide(latest, commits, lines(changed), trigger), nil
}

func git(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func lines(s string) []string {
	var ls []string
	for l := range strings.SplitSeq(s, "\n") {
		if l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

// writeOutput appends the plan as step outputs. The message spans lines, so
// it goes in the heredoc form with a delimiter it cannot contain.
func writeOutput(path string, plan Plan, sha string) error {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	delim := "EOF_" + hex.EncodeToString(buf)

	return appendFile(path, fmt.Sprintf("version=%s\nsha=%s\nmessage<<%s\n%s\n%s\n", plan.Version, sha, delim, plan.Message, delim))
}

func appendFile(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(s)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
