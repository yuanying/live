package main

import (
	"slices"
	"strings"
	"testing"
)

func TestLatestVersion(t *testing.T) {
	tests := []struct {
		name   string
		tags   []string
		want   string
		wantOK bool
	}{
		{
			name: "no tags",
		},
		{
			name: "only tags that are not vX.Y.Z",
			tags: []string{"v0.17.0.1", "release-1", "v1.2", "1.2.3", "v1.2.3-rc.1"},
		},
		{
			name:   "versions compare by number, not by string",
			tags:   []string{"v0.9.0", "v0.10.0", "v0.2.11"},
			want:   "v0.10.0",
			wantOK: true,
		},
		{
			name:   "a four-part tag above the latest version is ignored",
			tags:   []string{"v0.17.0", "v0.17.0.1", "v0.16.3"},
			want:   "v0.17.0",
			wantOK: true,
		},
		{
			name:   "live's tags",
			tags:   []string{"v1.0.0"},
			want:   "v1.0.0",
			wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LatestVersion(tt.tags)
			if ok != tt.wantOK {
				t.Fatalf("LatestVersion(%q) ok = %v, want %v", tt.tags, ok, tt.wantOK)
			}
			if ok && got.String() != tt.want {
				t.Errorf("LatestVersion(%q) = %s, want %s", tt.tags, got, tt.want)
			}
		})
	}
}

func TestDecide(t *testing.T) {
	latest := Version{Major: 1, Minor: 0, Patch: 0}
	renovate := func(subject string) Commit { return Commit{Author: RenovateAuthor, Subject: subject} }
	responder := func(subject string) Commit { return Commit{Author: VulnResponderAuthor, Subject: subject} }
	person := Commit{Hash: "0123456789abcdef0123456789abcdef01234567", Author: "OTSUKA, Yuanying", Subject: "Answer 418 as well"}

	tests := []struct {
		name        string
		commits     []Commit
		changed     []string
		wantRelease bool
		wantReason  string
		wantHeld    []Commit
	}{
		{
			name: "only Renovate, and the image's content changed",
			commits: []Commit{
				renovate("chore(deps): update golang docker tag to v1.27.4"),
				renovate("chore(deps): update gcr.io/distroless/static-debian13:latest docker digest to 0123abc"),
			},
			changed:     []string{"Dockerfile", "go.mod"},
			wantRelease: true,
		},
		{
			name: "a change to the source counts as the image's content",
			commits: []Commit{
				renovate("chore(deps): update module golang.org/x/net to v0.47.1"),
			},
			changed:     []string{"go.mod", "go.sum", "main.go"},
			wantRelease: true,
		},
		{
			name: "a commit by someone else leaves the release to a human",
			commits: []Commit{
				renovate("chore(deps): update go to v1.27.4"),
				person,
			},
			changed:    []string{"Dockerfile", "go.mod", "main.go"},
			wantReason: "not by renovate[bot] or fraction-vuln-responder[bot]",
			wantHeld:   []Commit{person},
		},
		{
			name: "only the vulnerability responder, and the image's content changed",
			commits: []Commit{
				responder("chore(deps): update golang.org/x/net to v0.47.2 for CVE-2026-12345"),
			},
			changed:     []string{"go.mod", "go.sum"},
			wantRelease: true,
		},
		{
			name: "Renovate and the vulnerability responder together",
			commits: []Commit{
				renovate("chore(deps): update golang docker tag to v1.27.4"),
				responder("chore(deps): update golang.org/x/net to v0.47.2 for CVE-2026-12345"),
			},
			changed:     []string{"Dockerfile", "go.mod", "go.sum"},
			wantRelease: true,
		},
		{
			name: "the vulnerability responder with a person's commit leaves the release to a human",
			commits: []Commit{
				responder("chore(deps): update golang.org/x/net to v0.47.2 for CVE-2026-12345"),
				person,
				renovate("chore(deps): update go to v1.27.4"),
			},
			changed:    []string{"Dockerfile", "go.mod", "go.sum", "main.go"},
			wantReason: "not by renovate[bot] or fraction-vuln-responder[bot]",
			wantHeld:   []Commit{person},
		},
		{
			name: "only Renovate, but only the workflows and renovate.json changed",
			commits: []Commit{
				renovate("chore(deps): update github-actions"),
				renovate("chore(config): migrate renovate config"),
			},
			changed:    []string{".github/workflows/image.yaml", ".github/workflows/release.yaml", "renovate.json"},
			wantReason: "nothing the image is built from",
		},
		{
			name: "only Renovate, but only the release plan changed",
			commits: []Commit{
				renovate("chore(deps): update module example.com/x to v1.0.1"),
			},
			changed:    []string{"hack/release-plan/plan.go"},
			wantReason: "nothing the image is built from",
		},
		{
			name:       "no commits since the tag",
			wantReason: "no commits since v1.0.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(latest, tt.commits, tt.changed, Weekly)
			if got.Release != tt.wantRelease {
				t.Fatalf("Decide().Release = %v, want %v (reason: %s)", got.Release, tt.wantRelease, got.Reason)
			}
			if got.Reason == "" {
				t.Error("Decide().Reason is empty; every outcome says why")
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("Decide().Reason = %q, want it to contain %q", got.Reason, tt.wantReason)
			}
			if !slices.Equal(got.Held, tt.wantHeld) {
				t.Errorf("Decide().Held = %v, want %v", got.Held, tt.wantHeld)
			}
			if !got.Release {
				if got.Version != "" || got.Message != "" {
					t.Errorf("Decide() without a release has Version %q and Message %q, want both empty", got.Version, got.Message)
				}
				return
			}
			if got.Version != "v1.0.1" {
				t.Errorf("Decide().Version = %q, want v1.0.1", got.Version)
			}
			if !strings.HasPrefix(got.Message, "v1.0.1: dependency updates\n\n") {
				t.Errorf("Decide().Message = %q, want it to open with the version and say these are dependency updates", got.Message)
			}
			for _, c := range tt.commits {
				if !strings.Contains(got.Message, "- "+c.Subject+"\n") {
					t.Errorf("Decide().Message = %q, want it to list %q", got.Message, c.Subject)
				}
			}
		})
	}
}

func TestIsImageContent(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"Dockerfile", true},
		{"go.mod", true},
		{"go.sum", true},
		{"main.go", true},
		{"handler.go", true},
		{".github/workflows/image.yaml", false},
		{".github/workflows/release.yaml", false},
		{"renovate.json", false},
		{"hack/release-plan/plan.go", false},
		{"README.md", false},
		{"main.go.orig", false},
	}
	for _, tt := range tests {
		if got := IsImageContent(tt.path); got != tt.want {
			t.Errorf("IsImageContent(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestDecideNamesWhatReleases(t *testing.T) {
	latest := Version{Major: 1, Minor: 0, Patch: 0}
	commits := []Commit{{Author: VulnResponderAuthor, Subject: "chore(deps): update golang.org/x/net to v0.47.2 for CVE-2026-12345"}}
	changed := []string{"go.mod", "go.sum"}

	tests := []struct {
		trigger Trigger
		want    string
		notWant string
	}{
		{trigger: Weekly, want: "weekly workflow", notWant: "vulnerability fix"},
		{trigger: VulnFix, want: "merge of a vulnerability fix", notWant: "weekly"},
	}
	for _, tt := range tests {
		t.Run(string(tt.trigger), func(t *testing.T) {
			got := Decide(latest, commits, changed, tt.trigger)
			if !got.Release {
				t.Fatalf("Decide().Release = false, want true (reason: %s)", got.Reason)
			}
			if !strings.Contains(got.Message, tt.want) {
				t.Errorf("Decide().Message = %q, want it to contain %q", got.Message, tt.want)
			}
			if strings.Contains(got.Message, tt.notWant) {
				t.Errorf("Decide().Message = %q, want it not to contain %q", got.Message, tt.notWant)
			}
		})
	}
}

func TestSummary(t *testing.T) {
	latest := Version{Major: 1, Minor: 0, Patch: 0}
	const sha = "fedcba9876543210fedcba9876543210fedcba98"
	person := Commit{Hash: "0123456789abcdef0123456789abcdef01234567", Author: "OTSUKA, Yuanying", Subject: "Answer 418 as well"}
	other := Commit{Hash: "89abcdef0123456789abcdef0123456789abcdef", Author: "someone", Subject: "Log the request"}
	commits := []Commit{
		{Hash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Author: VulnResponderAuthor, Subject: "chore(deps): update golang.org/x/net to v0.47.2 for CVE-2026-12345"},
		person,
		other,
	}

	plan := Decide(latest, commits, []string{"go.mod", "main.go"}, VulnFix)
	if plan.Release {
		t.Fatalf("Decide().Release = true, want false")
	}
	got := Summary(plan, sha)
	for _, want := range []string{
		"No release",
		plan.Reason,
		"0123456789ab", "OTSUKA, Yuanying", "Answer 418 as well",
		"89abcdef0123", "someone", "Log the request",
		"git tag -a v1.0.1 " + sha,
		"git push origin v1.0.1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Summary() = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "aaaaaaaaaaaa") {
		t.Errorf("Summary() = %q, want it not to list the responder's commit, which held nothing back", got)
	}

	if got := Summary(Decide(latest, nil, nil, VulnFix), sha); strings.Contains(got, "git tag") {
		t.Errorf("Summary() with no commits = %q, want no command to release nothing", got)
	}

	released := Decide(latest, commits[:1], []string{"go.mod"}, VulnFix)
	if got := Summary(released, sha); got != "" {
		t.Errorf("Summary() of a release = %q, want empty; the tag job reports the release", got)
	}
}
