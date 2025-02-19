package tasks

import (
	"fmt"
	"net/http"

	"github.com/merico-dev/lake/logger"
	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/core"
	"github.com/merico-dev/lake/plugins/github/models"
	"github.com/merico-dev/lake/utils"
	"gorm.io/gorm/clause"
)

type ApiPullRequestReviewResponse []PullRequestReview

type PullRequestReview struct {
	GithubId int `json:"id"`
	User     struct {
		Id    int
		Login string
	}
	Body        string
	State       string
	SubmittedAt core.Iso8601Time `json:"submitted_at"`
}

func CollectPullRequestReviews(owner string, repo string, scheduler *utils.WorkerScheduler, apiClient *GithubApiClient) error {
	var prs []models.GithubPullRequest
	lakeModels.Db.Find(&prs)
	for i := 0; i < len(prs); i++ {
		pr := (prs)[i]
		reviewErr := processPullRequestReviewsCollection(owner, repo, &pr, scheduler, apiClient)
		if reviewErr != nil {
			logger.Error("Could not collect PR Reviews", reviewErr)
			return reviewErr
		}
	}
	return nil
}
func processPullRequestReviewsCollection(owner string, repo string, pull *models.GithubPullRequest, scheduler *utils.WorkerScheduler, apiClient *GithubApiClient) error {
	getUrl := fmt.Sprintf("repos/%v/%v/pulls/%v/reviews", owner, repo, pull.Number)
	return apiClient.FetchWithPaginationAnts(getUrl, nil, 100, 1, scheduler,
		func(res *http.Response) error {
			githubApiResponse := &ApiPullRequestReviewResponse{}
			if res.StatusCode == 200 {
				err := core.UnmarshalResponse(res, githubApiResponse)
				if err != nil {
					logger.Error("Error: ", err)
					return err
				}
				for _, review := range *githubApiResponse {
					githubReviewer := &models.GithubReviewer{
						GithubId:      review.User.Id,
						Login:         review.User.Login,
						PullRequestId: pull.GithubId,
					}
					err = lakeModels.Db.Clauses(clause.OnConflict{
						UpdateAll: true,
					}).Create(&githubReviewer).Error
					if err != nil {
						logger.Error("Could not upsert: ", err)
					}
				}
			} else {
				fmt.Println("INFO: PR Review collection >>> res.Status: ", res.Status)
			}
			return nil
		})
}
