package git

import (
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// NewCommit specifies a new commit Action
type NewCommit struct {
}

// NewInstance returns a function that commits files to a git repository
func (n *NewCommit) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(opts interface{}) (force.Action, error) {
		return &CommitAction{
			options: opts,
		}, nil
	}
}

// CommitOptions specify details about the commit
type CommitOptions struct {
	// Directory is the path to the local git repostiory directory
	Directory string
	// PathSpecs is the pattern of files to include in this commit. If the
	// pathspec is empty, all currently staged files will be commited.
	PathSpecs []string
	// Message is the commit message
	Message string
	// AuthorName is the name of the author for this commit
	AuthorName string
	// AuthorEmail is the email address of the author for this commit
	AuthorEmail string
}

// CheckAndSetDefaults checks and sets default values
func (o *CommitOptions) CheckAndSetDefaults() error {
	if o.Directory == "" {
		o.Directory = "." // Current directory
	}

	if o.Message == "" {
		// Technically, git allows empty commit messages, but requires
		// the --allow-empty-message flag. I'm not aware that our git
		// library lets us set that flag, therefore I think it's safer
		// to just throw an error here.
		return trace.BadParameter("git commit message cannot be empty")
	}

	if o.AuthorName == "" || o.AuthorEmail == "" {
		return trace.BadParameter("commit author needs to be set")
	}

	return nil
}

// CommitAction returns new commit actions
type CommitAction struct {
	options interface{}
}

func (p *CommitAction) Type() interface{} {
	return ""
}

// MarshalCode marshals the action into code representation
func (c *CommitAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      KeyCommit,
		Args:    []interface{}{c.options},
	}
	return call.MarshalCode(ctx)
}

// Eval commits files to the git repository
func (p *CommitAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	var opts CommitOptions
	if err := force.EvalInto(ctx, p.options, &opts); err != nil {
		return nil, trace.Wrap(err)
	}

	if err := opts.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	repo, err := git.PlainOpen(opts.Directory)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for _, pathSpec := range opts.PathSpecs {
		if err := w.AddGlob(pathSpec); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	hash, err := w.Commit(opts.Message, &git.CommitOptions{
		Author: &object.Signature{
			Name: opts.AuthorName,
			Email: opts.AuthorEmail,
			When: time.Now(),
		},
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return hash.String(), nil
}
