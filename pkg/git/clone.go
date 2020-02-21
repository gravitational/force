package git

import (
	"os"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	git "gopkg.in/src-d/go-git.v4"
	gitconfig "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// Clone executes inner action and posts result of it's execution
// to github
func Clone(repo interface{}) (force.Action, error) {
	return &CloneAction{
		repo: repo,
	}, nil
}

// NewClone creates functions cloning repositories
type NewClone struct {
}

// NewInstance returns a function that wraps underlying action
// and tracks the result, posting the result back
func (n *NewClone) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Clone
}

// CloneAction clones repository
type CloneAction struct {
	repo interface{}
}

func (p *CloneAction) Type() interface{} {
	return ""
}

func (p *CloneAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return nil, trace.NotFound("initialize Git plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

	var repo Repo
	if err := force.EvalInto(ctx, p.repo, &repo); err != nil {
		return nil, trace.Wrap(err)
	}

	log := force.Log(ctx)
	if repo.Into == "" {
		return nil, trace.BadParameter("got empty Into variable")
	}
	fi, err := os.Stat(repo.Into)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	if !fi.IsDir() {
		return nil, trace.BadParameter("Into variable is not an existing directory")
	}

	log.Infof("Cloning repository %v into %v.", repo.URL, repo.Into)
	start := time.Now()

	auth, err := plugin.cfg.Auth()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	r, err := git.PlainClone(repo.Into, false, &git.CloneOptions{
		Auth: auth,
		URL:  string(repo.URL),
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	err = r.Fetch(&git.FetchOptions{
		Auth:     auth,
		RefSpecs: []gitconfig.RefSpec{"refs/*:refs/*", "HEAD:refs/heads/HEAD"},
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log.Infof("Cloned %v into %v in %v.", repo.URL, repo.Into, time.Now().Sub(start))

	if repo.Hash != "" {
		w, err := r.Worktree()
		if err != nil {
			return nil, trace.Wrap(err)
		}

		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(repo.Hash),
		})
		if err != nil {
			return nil, trace.Wrap(err)
		}

		log.Infof("Checked out repository %v commit %v.", repo.URL, repo.Hash)
	}

	if repo.Tag != "" {
		w, err := r.Worktree()
		if err != nil {
			return nil, trace.Wrap(err)
		}

		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewTagReferenceName(repo.Tag),
		})
		if err != nil {
			return nil, trace.BadParameter("failed to clone tag %v: %v", repo.Tag, err)
		}

		log.Infof("Checked out repository %v tag %v.", repo.URL, repo.Tag)
	}

	if repo.Branch != "" {
		w, err := r.Worktree()
		if err != nil {
			return nil, trace.Wrap(err)
		}

		err = w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName(repo.Branch),
		})
		if err != nil {
			return nil, trace.BadParameter("failed to clone branch %v: %v", repo.Branch, err)
		}

		log.Infof("Checked out repository %v branch %v.", repo.URL, repo.Branch)
	}

	for i, subName := range repo.Submodules {
		if subName == "" {
			return nil, trace.BadParameter("got empty submodule name at %v", i)
		}
		w, err := r.Worktree()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		log.Infof("Updating submodule %v.", subName)
		sub, err := w.Submodule(subName)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		err = sub.UpdateContext(ctx, &git.SubmoduleUpdateOptions{
			Init: true,
			Auth: auth,
		})
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	return repo.URL, nil
}

// MarshalCode marshals action into code representation
func (c *CloneAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      Clone,
		Args:    []interface{}{c.repo},
	}
	return call.MarshalCode(ctx)
}

