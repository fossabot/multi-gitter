package multigitter

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	log "github.com/sirupsen/logrus"

	"github.com/lindell/multi-gitter/internal/domain"
	"github.com/lindell/multi-gitter/internal/git"
	"github.com/lindell/multi-gitter/internal/multigitter/repocounter"
)

// Printer contains fields to be able to do the print command
type Printer struct {
	VersionController VersionController

	ScriptPath string // Must be absolute path
	Arguments  []string
	Token      string

	Stdout io.Writer
	Stderr io.Writer

	Concurrent int
}

func (r Printer) Output(ctx context.Context) error {
	repos, err := r.VersionController.GetRepositories(ctx)
	if err != nil {
		return err
	}

	rc := repocounter.NewCounter()
	defer func() {
		if info := rc.Info(); info != "" {
			fmt.Fprint(log.StandardLogger().Out, info)
		}
	}()

	log.Infof("Running on %d repositories", len(repos))

	runInParallel(func(i int) {
		logger := log.WithField("repo", repos[i].FullName())
		err := r.runSingleRepo(ctx, repos[i])
		if err != nil {
			if err != errAborted {
				logger.Info(err)
			}
			rc.AddError(err, repos[i])
			return
		}

		rc.AddSuccess(repos[i])
	}, len(repos), r.Concurrent)

	return nil
}

func (r Printer) runSingleRepo(ctx context.Context, repo domain.Repository) error {
	if ctx.Err() != nil {
		return errAborted
	}

	log := log.WithField("repo", repo.FullName())
	log.Info("Cloning and running script")

	tmpDir, err := ioutil.TempDir(os.TempDir(), "multi-git-changer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	sourceController := &git.Git{
		Directory: tmpDir,
		Repo:      repo.URL(r.Token),
	}

	err = sourceController.Clone()
	if err != nil {
		return err
	}

	// Run the command that might or might not change the content of the repo
	// If the command return a non zero exit code, abort.
	cmd := exec.Command(r.ScriptPath, r.Arguments...)
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("REPOSITORY=%s", repo.FullName()),
	)

	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr

	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}