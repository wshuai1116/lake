package tasks

import (
	"fmt"
	"github.com/merico-dev/lake/models/domainlayer/ticket"
	"strings"

	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/jira/models"
)

func EnrichIssues(source *models.JiraSource, boardId uint64) (err error) {
	jiraIssue := &models.JiraIssue{}

	// prepare getStdType function
	var typeMappingRows []*models.JiraIssueTypeMapping
	err = lakeModels.Db.Find(&typeMappingRows, "source_id = ?", source.ID).Error
	if err != nil {
		return err
	}
	typeMappings := make(map[string]string)
	for _, typeMappingRow := range typeMappingRows {
		typeMappings[typeMappingRow.UserType] = typeMappingRow.StandardType
	}
	getStdType := func(userType string) string {
		stdType := typeMappings[userType]
		if stdType == "" {
			return strings.ToUpper(userType)
		}
		return strings.ToUpper(stdType)
	}
	// prepare getStdStatus function
	var statusMappingRows []*models.JiraIssueStatusMapping
	err = lakeModels.Db.Find(&statusMappingRows, "source_id = ?", source.ID).Error
	if err != nil {
		return err
	}
	statusMappings := make(map[string]string)
	makeStatusMappingKey := func(userType string, userStatus string) string {
		return fmt.Sprintf("%v:%v", userType, userStatus)
	}
	for _, statusMappingRow := range statusMappingRows {
		k := makeStatusMappingKey(statusMappingRow.UserType, statusMappingRow.UserStatus)
		statusMappings[k] = statusMappingRow.StandardStatus
	}
	getStdStatus := func(statusKey string) string {
		if statusKey == "done" {
			return ticket.DONE
		} else if statusKey == "new" {
			return ticket.TODO
		} else {
			return ticket.IN_PROGRESS
		}
	}

	// select all issues belongs to the board
	cursor, err := lakeModels.Db.Model(jiraIssue).
		Select("jira_issues.*").
		Joins("left join jira_board_issues on jira_board_issues.issue_id = jira_issues.issue_id").
		Where("jira_board_issues.board_id = ? AND jira_board_issues.source_id = ?", boardId, source.ID).
		Rows()
	if err != nil {
		return err
	}
	defer cursor.Close()

	// iterate all rows
	for cursor.Next() {
		err = lakeModels.Db.ScanRows(cursor, jiraIssue)
		if err != nil {
			return err
		}
		if jiraIssue.ResolutionDate != nil {
			jiraIssue.LeadTimeMinutes = uint(jiraIssue.ResolutionDate.Unix()-jiraIssue.Created.Unix()) / 60
		}
		jiraIssue.StdStoryPoint = uint(jiraIssue.StoryPoint)
		jiraIssue.StdType = getStdType(jiraIssue.Type)
		jiraIssue.StdStatus = getStdStatus(jiraIssue.StatusKey)
		// assuming remaining estimate could be negative; TODO: make sure of it
		jiraIssue.SpentMinutes = jiraIssue.AggregateEstimateMinutes - jiraIssue.RemainingEstimateMinutes
		err = lakeModels.Db.Save(jiraIssue).Error
		if err != nil {
			return err
		}
	}
	return nil
}
