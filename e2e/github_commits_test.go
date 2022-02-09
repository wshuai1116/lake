package e2e

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
)

// This test should only run once main_test is complete and ready

type GithubCommit struct {
	Sha int `json:"sha"`
}

func TestGitHubCommits(t *testing.T) {
	var commits []GithubCommit
	db, err := InitializeDb()
	assert.Nil(t, err)
	if err != nil {
		log.Fatal(err)
	}
	rows, err := db.Query("SELECT github_id FROM github_commits")
	if err != nil {
		fmt.Println("KEVIN >>> err", err)
	}
	assert.Nil(t, err)
	defer rows.Close()
	for rows.Next() {
		var commit GithubCommit
		if err := rows.Scan(&commit.Sha); err != nil {
			panic(err)
		}
		commits = append(commits, commit)
	}
	assert.Equal(t, len(commits) == 0, true)
}
