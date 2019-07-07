/*
MIT License

Copyright (c) 2018 Telia Company

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package github

import (
	"time"

	"github.com/gravitational/trace"
	"github.com/shurcooL/githubv4"
)

// Source represents the configuration for the resource.
type Source struct {
	// Repo is a repository to watch
	Repo string
	// Token is a github security access token
	Token string
	// Branch is a branch to check PRs against
	Branch string
}

const (
	// MasterBranch is a default github master branch to watch
	MasterBranch = "master"
)

// CheckAndSetDefaults checks and sets default values
func (s *Source) CheckAndSetDefaults() error {
	if s.Token == "" {
		return trace.BadParameter("set Source{Token:``} parameter")
	}
	if s.Repo == "" {
		return trace.BadParameter("set Source{Token:``} parameter")
	}
	if s.Branch == "" {
		s.Branch = MasterBranch
	}
	return nil
}

// PullRequests is a list of pull request
type PullRequests []PullRequest

func (p PullRequests) Len() int {
	return len(p)
}

func (p PullRequests) Less(i, j int) bool {
	return p[j].LastUpdated().After(p[i].LastUpdated())
}

func (p PullRequests) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// PullRequest represents a pull request and includes the last commit
// and the last comment
type PullRequest struct {
	PullRequestObject
	LastCommit  CommitObject
	LastComment CommentObject
}

// LastUpdated returns either the last commit date
// or the last comment date whatever happened later
func (p *PullRequest) LastUpdated() time.Time {
	a := p.LastCommit.CommittedDate.Time
	b := p.LastComment.CreatedAt.Time
	if a.After(b) {
		return a
	}
	return b
}

// PullRequestObject represents the GraphQL commit node.
// https://developer.github.com/v4/object/pullrequest/
type PullRequestObject struct {
	ID          string
	Number      int
	Title       string
	URL         string
	BaseRefName string
	HeadRefName string
	Repository  struct {
		URL string
	}
	IsCrossRepository bool
}

// CommitObject represents the GraphQL commit node.
// https://developer.github.com/v4/object/commit/
type CommitObject struct {
	ID            string
	OID           string
	CommittedDate githubv4.DateTime
	Message       string
	Author        struct {
		User struct {
			Login string
		}
	}
}

// CommentObject represents the GraphQL commit node.
// https://developer.github.com/v4/object/commit/
type CommentObject struct {
	ID        string
	CreatedAt githubv4.DateTime
	UpdatedAt githubv4.DateTime
	Body      string
	Author    struct {
		Login string
	}
}
