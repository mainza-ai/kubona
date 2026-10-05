package autoresearch

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// BranchManager manages counterfactual git branches for autonomous research experiments.
// Each iteration operates on an ephemeral branch `experiment/<harness_id>/<iter>`.
// Discards are rolled back by deleting the branch; promotions are committed and merged.
type BranchManager struct {
	mu              sync.Mutex
	repoDir         string
	baseBranch      string
	artifactRelPath string
}

// NewBranchManager creates a new BranchManager targeting repoDir with baseBranch and artifactRelPath.
func NewBranchManager(repoDir string, baseBranch string, artifactRelPath string) (*BranchManager, error) {
	if repoDir == "" {
		return nil, fmt.Errorf("repoDir cannot be empty")
	}
	if baseBranch == "" {
		baseBranch = "main"
	}
	if artifactRelPath == "" {
		artifactRelPath = "artifact.py"
	}

	bm := &BranchManager{
		repoDir:         repoDir,
		baseBranch:      baseBranch,
		artifactRelPath: artifactRelPath,
	}

	// Verify repoDir is a valid git repo
	if _, err := bm.runGit("rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("repoDir %s is not a valid git repository: %w", repoDir, err)
	}

	return bm, nil
}

func (bm *BranchManager) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = bm.repoDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// CurrentBranch returns the name of the currently checked out branch.
func (bm *BranchManager) CurrentBranch() (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	return bm.runGit("rev-parse", "--abbrev-ref", "HEAD")
}

// CreateExperimentBranch creates and checks out a new ephemeral branch for an iteration.
func (bm *BranchManager) CreateExperimentBranch(harnessID string, iter int) (string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Ensure we start cleanly from baseBranch
	if _, err := bm.runGit("checkout", bm.baseBranch); err != nil {
		return "", fmt.Errorf("failed to checkout base branch %s: %w", bm.baseBranch, err)
	}

	branchName := fmt.Sprintf("experiment/%s/iter-%d", sanitizeBranchName(harnessID), iter)

	// Check if branch already exists, delete if so
	_, _ = bm.runGit("branch", "-D", branchName)

	if _, err := bm.runGit("checkout", "-b", branchName); err != nil {
		return "", fmt.Errorf("failed to create experiment branch %s: %w", branchName, err)
	}

	return branchName, nil
}

// WriteArtifact writes the candidate mutated artifact to the repository working directory.
func (bm *BranchManager) WriteArtifact(content string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	targetPath := filepath.Join(bm.repoDir, bm.artifactRelPath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create artifact directory: %w", err)
	}

	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write artifact file: %w", err)
	}
	return nil
}

// CommitAndPromote commits the experiment on the branch, merges it into baseBranch, and optionally tags it.
func (bm *BranchManager) CommitAndPromote(branchName string, commitMsg string, tagName string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Stage artifact file
	if _, err := bm.runGit("add", bm.artifactRelPath); err != nil {
		return fmt.Errorf("failed to stage artifact %s: %w", bm.artifactRelPath, err)
	}

	// Commit if there are changes
	status, err := bm.runGit("status", "--porcelain")
	if err != nil {
		return fmt.Errorf("failed to get git status: %w", err)
	}

	if strings.TrimSpace(status) != "" {
		if _, err := bm.runGit("commit", "-m", commitMsg); err != nil {
			return fmt.Errorf("failed to commit artifact: %w", err)
		}
	}

	// Checkout base branch
	if _, err := bm.runGit("checkout", bm.baseBranch); err != nil {
		return fmt.Errorf("failed to checkout base branch %s: %w", bm.baseBranch, err)
	}

	// Merge experiment branch
	if _, err := bm.runGit("merge", "--no-ff", "-m", fmt.Sprintf("Merge %s into %s", branchName, bm.baseBranch), branchName); err != nil {
		return fmt.Errorf("failed to merge %s into %s: %w", branchName, bm.baseBranch, err)
	}

	// Optional tag
	if tagName != "" {
		_ = bm.tag(tagName, commitMsg)
	}

	// Clean up experiment branch
	_, _ = bm.runGit("branch", "-D", branchName)

	return nil
}

// DiscardBranch rolls back changes and deletes the experiment branch, returning to baseBranch.
func (bm *BranchManager) DiscardBranch(branchName string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// Force checkout back to baseBranch
	if _, err := bm.runGit("checkout", "-f", bm.baseBranch); err != nil {
		return fmt.Errorf("failed to revert to base branch %s: %w", bm.baseBranch, err)
	}

	// Clean any untracked files
	_, _ = bm.runGit("clean", "-fd")

	// Delete experiment branch
	_, err := bm.runGit("branch", "-D", branchName)
	return err
}

func (bm *BranchManager) tag(tagName, message string) error {
	_, err := bm.runGit("tag", "-a", tagName, "-m", message)
	return err
}

func sanitizeBranchName(name string) string {
	res := strings.ReplaceAll(name, " ", "-")
	res = strings.ReplaceAll(res, "/", "-")
	res = strings.ReplaceAll(res, ":", "-")
	return res
}
