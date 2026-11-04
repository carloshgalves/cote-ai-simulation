package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual CI shell block against a synthetic merge. A whitespace
// defect already present on the PR base is outside the proposed change; a new
// defect in the PR must still fail. Git operations stay in disposable fixtures.
func TestDecisionWorkflowWhitespaceUsesPullRequestBase(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/csfrv1-4-implementation.yml")
	if err != nil {
		t.Fatal(err)
	}
	section := strings.SplitN(string(workflow), "- name: Diff whitespace check", 2)
	if len(section) != 2 {
		t.Fatal("whitespace check step missing")
	}
	if !strings.Contains(string(workflow), "fetch-depth: 2") ||
		!strings.Contains(section[1], "CSF_EVENT_NAME: ${{ github.event_name }}") ||
		!strings.Contains(section[1], "CSF_PR_HEAD: ${{ github.event.pull_request.head.sha }}") {
		t.Fatal("whitespace step requires the synthetic merge parents and expected PR head")
	}
	block := strings.SplitN(section[1], "run: |\n", 2)
	if len(block) != 2 {
		t.Fatal("whitespace shell block missing")
	}
	var lines []string
	for _, line := range strings.Split(block[1], "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "          ") {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	if len(lines) == 0 {
		t.Fatal("empty whitespace shell block")
	}
	for _, brokenFeature := range []bool{false, true} {
		t.Run(map[bool]string{false: "unrelated-base-whitespace", true: "feature-whitespace"}[brokenFeature], func(t *testing.T) {
			dir := t.TempDir()
			gitEnv := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
			git := func(args ...string) string {
				command := exec.Command("git", append([]string{"-c", "user.name=CI fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
				command.Dir = dir
				command.Env = gitEnv
				out, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("fixture git %v: %v\n%s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			write := func(name, content string) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			git("init", "--quiet", "--initial-branch=feature")
			write("initial.txt", "initial\n")
			git("add", ".")
			git("commit", "--quiet", "-m", "initial")
			initial := git("rev-parse", "HEAD")
			git("switch", "--quiet", "-c", "base")
			write("unrelated.txt", "inherited whitespace \n")
			git("add", ".")
			git("commit", "--quiet", "-m", "unrelated base change")
			base := git("rev-parse", "HEAD")
			git("switch", "--quiet", "feature")
			content := "proposed change\n"
			if brokenFeature {
				content = "proposed change \n"
			}
			write("feature.txt", content)
			git("add", ".")
			git("commit", "--quiet", "-m", "feature change")
			feature := git("rev-parse", "HEAD")
			git("switch", "--quiet", "base")
			git("merge", "--quiet", "--no-ff", "feature", "-m", "synthetic PR merge")
			if git("rev-parse", "HEAD^1") != base || git("rev-parse", "HEAD^2") != feature {
				t.Fatal("fixture does not match GitHub's merge parent order")
			}
			git("remote", "add", "origin", dir)
			// Translate the historical push SHA into this fixture's ancestor.
			script := strings.ReplaceAll(strings.Join(lines, "\n"), "43574722e3f22eb629da47e17bf573dad4a68369", initial)
			command := exec.Command("bash", "-e", "-c", script)
			command.Dir = dir
			// Model the observed stale event SHA despite a newer merge base.
			command.Env = append(gitEnv, "CSF_DIFF_BASE="+initial, "CSF_EVENT_NAME=pull_request", "CSF_PR_HEAD="+feature)
			out, err := command.CombinedOutput()
			if !brokenFeature && err != nil {
				t.Fatalf("unrelated base whitespace rejected the PR: %v\n%s", err, out)
			}
			if brokenFeature && (err == nil || !strings.Contains(string(out), "feature.txt")) {
				t.Fatalf("feature whitespace not rejected: %v\n%s", err, out)
			}
			if !brokenFeature {
				command = exec.Command("bash", "-e", "-c", script)
				command.Dir = dir
				command.Env = append(gitEnv, "CSF_DIFF_BASE="+initial, "CSF_EVENT_NAME=pull_request", "CSF_PR_HEAD="+initial)
				out, err = command.CombinedOutput()
				if err == nil || !strings.Contains(string(out), "PR head") {
					t.Fatalf("wrong synthetic merge head accepted: %v\n%s", err, out)
				}
			}
		})
	}
}
