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

	"github.com/shurcooL/githubv4"
)

const (
	// MasterBranch is a default github master branch to watch
	MasterBranch = "master"
)

type pullRequestUpdate struct {
	PullRequest
	newCommit  bool
	newComment bool
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

// UserObject is graphql user node
type UserObject struct {
	Login string
}

// Branch represents a branch that was updated
type Branch struct {
	RefObject
	CommitObject
}

// RefObject represents the GraphQL ref node
// https://developer.github.com/v4/object/ref/
type RefObject struct {
	ID     string
	Name   string
	Prefix string
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
