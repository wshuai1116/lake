package models

import (
	"context"

	"github.com/merico-dev/lake/models/domainlayer/code"
)

type Store interface {
	RepoCommits(repoCommit *code.RepoCommit) error
	Commits(commit *code.Commit) error
	Refs(ref *code.Ref) error
	CommitFiles(file *code.CommitFile) error
	CommitParents(pp []*code.CommitParent) error
	Close() error
}

type Parser interface {
	RemoteRepo(ctx context.Context, url, repoId, proxy string) error
	LocalRepo(ctx context.Context, repoPath, repoId string) error
}