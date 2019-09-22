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
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/google/go-github/github"
	"github.com/gravitational/trace"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// GithubClient for handling requests to the Github V3 and V4 APIs.
type GithubClient struct {
	V3 *github.Client
	V4 *githubv4.Client
}

// newGithubClient creates new github client
func newGithubClient(ctx context.Context, cfg Config) (*GithubClient, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: string(cfg.Token)},
	))
	return &GithubClient{
		V3: github.NewClient(client),
		V4: githubv4.NewClient(client),
	}, nil
}

// GetTeamMembers returns all team members for a given org
func (m *GithubClient) GetTeamMembers(ctx context.Context, org, slug string) ([]UserObject, error) {
	var query struct {
		Organization struct {
			Team struct {
				Members struct {
					Nodes []struct {
						UserObject
					}
					PageInfo struct {
						EndCursor   githubv4.String
						HasNextPage bool
					}
				} `graphql:"members(first:$memberFirst,after:$memberCursor)"`
			} `graphql:"team(slug:$teamSlug)"`
		} `graphql:"organization(login:$orgName)"`
	}

	vars := map[string]interface{}{
		"orgName":      githubv4.String(org),
		"teamSlug":     githubv4.String(slug),
		"memberFirst":  githubv4.Int(100),
		"memberCursor": (*githubv4.String)(nil),
	}

	var users []UserObject
	for {
		if err := m.V4.Query(ctx, &query, vars); err != nil {
			return nil, trace.Wrap(err)
		}
		for _, member := range query.Organization.Team.Members.Nodes {
			users = append(users, member.UserObject)
		}
		if !query.Organization.Team.Members.PageInfo.HasNextPage {
			break
		}
		vars["prCursor"] = query.Organization.Team.Members.PageInfo.EndCursor
	}
	return users, nil
}

// GetPullRequestComments returns PR comments
func (m *GithubClient) GetPullRequestComments(ctx context.Context, repo Repository, prNumber int) ([]CommentObject, error) {
	var query struct {
		Repository struct {
			PullRequest struct {
				Comments struct {
					Edges []struct {
						Node struct {
							CommentObject
						}
					}
					PageInfo struct {
						EndCursor   githubv4.String
						HasNextPage bool
					}
				} `graphql:"comments(first:$commentsFirst, after: $commentsCursor)"`
			} `graphql:"pullRequest(number: $prNumber)"`
		} `graphql:"repository(owner:$repositoryOwner,name:$repositoryName)"`
	}

	vars := map[string]interface{}{
		"repositoryOwner": githubv4.String(repo.Owner),
		"repositoryName":  githubv4.String(repo.Name),
		"prNumber":        githubv4.Int(prNumber),
		"commentsFirst":   githubv4.Int(100),
		"commentsCursor":  (*githubv4.String)(nil),
	}

	var comments []CommentObject
	for {
		if err := m.V4.Query(ctx, &query, vars); err != nil {
			return nil, err
		}
		for _, comment := range query.Repository.PullRequest.Comments.Edges {
			comments = append(comments, comment.Node.CommentObject)
		}
		if !query.Repository.PullRequest.Comments.PageInfo.HasNextPage {
			break
		}
		vars["commentsCursor"] = query.Repository.PullRequest.Comments.PageInfo.EndCursor
	}
	return comments, nil
}

// GetOpenPullRequests gets the last commit on all open pull requests.
func (m *GithubClient) GetOpenPullRequests(ctx context.Context, repo Repository) ([]PullRequest, error) {
	var query struct {
		Repository struct {
			PullRequests struct {
				Edges []struct {
					Node struct {
						PullRequestObject
						Commits struct {
							Edges []struct {
								Node struct {
									Commit CommitObject
								}
							}
						} `graphql:"commits(last:$commitsLast)"`
						Comments struct {
							Edges []struct {
								Node struct {
									CommentObject
								}
							}
						} `graphql:"comments(last:$commentsLast)"`
					}
				}
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"pullRequests(first:$prFirst,states:$prStates,after:$prCursor)"`
		} `graphql:"repository(owner:$repositoryOwner,name:$repositoryName)"`
	}

	vars := map[string]interface{}{
		"repositoryOwner": githubv4.String(repo.Owner),
		"repositoryName":  githubv4.String(repo.Name),
		"prFirst":         githubv4.Int(100),
		"prStates":        []githubv4.PullRequestState{githubv4.PullRequestStateOpen},
		"prCursor":        (*githubv4.String)(nil),
		"commitsLast":     githubv4.Int(1),
		"commentsLast":    githubv4.Int(1),
	}

	var pullRequests []PullRequest
	for {
		if err := m.V4.Query(ctx, &query, vars); err != nil {
			return nil, err
		}
		for _, pr := range query.Repository.PullRequests.Edges {
			pullRequest := PullRequest{
				PullRequestObject: pr.Node.PullRequestObject,
			}
			for _, commit := range pr.Node.Commits.Edges {
				pullRequest.LastCommit = commit.Node.Commit
			}
			for _, comment := range pr.Node.Comments.Edges {
				pullRequest.LastComment = comment.Node.CommentObject
			}
			pullRequests = append(pullRequests, pullRequest)
		}
		if !query.Repository.PullRequests.PageInfo.HasNextPage {
			break
		}
		vars["prCursor"] = query.Repository.PullRequests.PageInfo.EndCursor
	}
	return pullRequests, nil
}

// GetBranches gets the last commit on branches with changes matching the path
func (m *GithubClient) GetBranches(ctx context.Context, repo Repository, path string) ([]Branch, error) {
	var query struct {
		Repository struct {
			Refs struct {
				Nodes []struct {
					RefObject
					Target struct {
						OnCommit struct {
							CommitObject
							History struct {
								Edges []struct {
									Node struct {
										CommitObject
									}
								}
							} `graphql:"history(first:1,path:$path)"`
						} `graphql:"... on Commit"`
					} `graphql:"target"`
				}
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"refs(first:$refFirst,after:$refCursor,refPrefix:$refPrefix)"`
		} `graphql:"repository(owner:$repositoryOwner,name:$repositoryName)"`
	}

	vars := map[string]interface{}{
		"repositoryOwner": githubv4.String(repo.Owner),
		"repositoryName":  githubv4.String(repo.Name),
		"refFirst":        githubv4.Int(100),
		"refPrefix":       githubv4.String("refs/heads/"),
		"refCursor":       (*githubv4.String)(nil),
		"path":            (*githubv4.String)(nil),
	}

	if path != "" {
		vars["path"] = githubv4.String(path)
	}

	var branches []Branch
	for {
		if err := m.V4.Query(ctx, &query, vars); err != nil {
			return nil, err
		}
		for _, ref := range query.Repository.Refs.Nodes {
			lastCommit := ref.Target.OnCommit.CommitObject
			// no commits matching the path, or the last commit != last commit matching the path (no updates to the path)
			if len(ref.Target.OnCommit.History.Edges) == 0 || ref.Target.OnCommit.History.Edges[0].Node.OID != lastCommit.OID {
				continue
			}
			branch := Branch{
				RefObject:    ref.RefObject,
				CommitObject: ref.Target.OnCommit.History.Edges[0].Node.CommitObject,
			}
			branches = append(branches, branch)
		}
		if !query.Repository.Refs.PageInfo.HasNextPage {
			break
		}
		vars["refCursor"] = query.Repository.Refs.PageInfo.EndCursor
	}
	return branches, nil
}

// ListModifiedFiles in a pull request (not supported by V4 API).
func (m *GithubClient) ListModifiedFiles(repo Repository, prNumber int) ([]string, error) {
	var files []string

	opt := &github.ListOptions{
		PerPage: 100,
	}
	for {
		result, response, err := m.V3.PullRequests.ListFiles(
			context.TODO(),
			repo.Owner,
			repo.Name,
			prNumber,
			opt,
		)
		if err != nil {
			return nil, err
		}
		for _, f := range result {
			files = append(files, *f.Filename)
		}
		if response.NextPage == 0 {
			break
		}
		opt.Page = response.NextPage
	}
	return files, nil
}

// PostComment to a pull request or issue.
func (m *GithubClient) PostComment(repo Repository, prNumber, comment string) error {
	pr, err := strconv.Atoi(prNumber)
	if err != nil {
		return trace.Wrap(err, "failed to convert pull request number to int")
	}

	_, _, err = m.V3.Issues.CreateComment(
		context.TODO(),
		repo.Owner,
		repo.Name,
		pr,
		&github.IssueComment{
			Body: github.String(comment),
		},
	)
	return err
}

// UpdateCommitStatus for a given commit (not supported by V4 API).
func (m *GithubClient) UpdateCommitStatus(repo Repository, commitRef, baseContext, statusContext, status, targetURL, description string) error {
	if baseContext == "" {
		baseContext = "force"
	}

	if statusContext == "" {
		statusContext = "status"
	}

	if targetURL == "" {
		targetURL = strings.Join([]string{os.Getenv("ATC_EXTERNAL_URL"), "builds", os.Getenv("BUILD_ID")}, "/")
	}

	if description == "" {
		description = fmt.Sprintf("Force CI build %s", status)
	}

	_, _, err := m.V3.Repositories.CreateStatus(
		context.TODO(),
		repo.Owner,
		repo.Name,
		commitRef,
		&github.RepoStatus{
			State:       github.String(strings.ToLower(status)),
			TargetURL:   github.String(targetURL),
			Description: github.String(description),
			Context:     github.String(path.Join(baseContext, statusContext)),
		},
	)
	return err
}

func parseRepository(s string) (string, string, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", "", trace.BadParameter("malformed repository %q format, expected owner/repo", s)
	}
	return parts[0], parts[1], nil
}
