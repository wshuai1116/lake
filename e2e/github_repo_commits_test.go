package e2e

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

// This test should only run once main_test is complete and ready

type GithubRepoCommits struct {
	GithubId string `json:"github_id"`
}

func TestGitHubGithubRepoCommitss(t *testing.T) {
	var repos []GithubRepoCommits
	db, err := InitializeDb()
	assert.Nil(t, err)
	if err != nil {
		log.Fatal(err)
	}
	rows, err := db.Query("Select author_id from github_commits gc JOIN github_repo_commits grc ON grc.commit_sha = gc.sha where authored_date < '2021-12-25 04:40:11.000';")
	if err != nil {
		fmt.Println("KEVIN >>> err", err)
	}
	assert.Nil(t, err)
	defer rows.Close()
	for rows.Next() {
		var repo GithubRepoCommits
		if err := rows.Scan(&repo.GithubId); err != nil {
			panic(err)
		}
		repos = append(repos, repo)
	}
	assert.Equal(t, len(repos), 1093)
}
