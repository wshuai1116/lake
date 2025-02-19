package tasks

import (
	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/models/domainlayer/code"
	"github.com/merico-dev/lake/models/domainlayer/didgen"
	"github.com/merico-dev/lake/plugins/gitlab/models"
	"gorm.io/gorm/clause"
)

func ConvertCommits(projectId int) error {
	// select all commits belongs to the project
	cursor, err := lakeModels.Db.Table("gitlab_commits gc").
		Joins(`left join gitlab_project_commits gpc on (
			gpc.commit_sha = gc.sha
		)`).
		Select("gc.*").
		Where("gpc.gitlab_project_id = ?", projectId).
		Rows()
	if err != nil {
		return err
	}
	defer cursor.Close()

	// TODO: adopt batch indate operation
	userDidGen := didgen.NewDomainIdGenerator(&models.GitlabUser{})
	repoCommit := &code.RepoCommit{
		RepoId: didgen.NewDomainIdGenerator(&models.GitlabProject{}).Generate(projectId),
	}
	gitlabCommit := &models.GitlabCommit{}
	commit := &code.Commit{}
	for cursor.Next() {
		err = lakeModels.Db.ScanRows(cursor, gitlabCommit)
		if err != nil {
			return err
		}
		// convert commit
		commit.Sha = gitlabCommit.Sha
		commit.Message = gitlabCommit.Message
		commit.Additions = gitlabCommit.Additions
		commit.Deletions = gitlabCommit.Deletions
		commit.AuthorId = userDidGen.Generate(gitlabCommit.AuthorEmail)
		commit.AuthorName = gitlabCommit.AuthorName
		commit.AuthorEmail = gitlabCommit.AuthorEmail
		commit.AuthoredDate = gitlabCommit.AuthoredDate
		commit.CommitterName = gitlabCommit.CommitterName
		commit.CommitterEmail = gitlabCommit.CommitterEmail
		commit.CommittedDate = gitlabCommit.CommittedDate
		commit.CommitterId = userDidGen.Generate(gitlabCommit.AuthorEmail)
		err := lakeModels.Db.Clauses(clause.OnConflict{UpdateAll: true}).Create(commit).Error
		if err != nil {
			return err
		}
		// convert repo / commits relationship
		repoCommit.CommitSha = gitlabCommit.Sha
		err = lakeModels.Db.Clauses(clause.OnConflict{DoNothing: true}).Create(repoCommit).Error
		if err != nil {
			return err
		}
	}
	return nil
}
