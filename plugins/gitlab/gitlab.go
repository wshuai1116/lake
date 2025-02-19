package main // must be main for plugin entry point

import (
	"context"
	"fmt"
	errors "github.com/merico-dev/lake/errors"
	"os"
	"strconv"

	"github.com/merico-dev/lake/logger" // A pseudo type for Plugin Interface implementation
	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/core"
	"github.com/merico-dev/lake/plugins/gitlab/api"
	"github.com/merico-dev/lake/plugins/gitlab/models"
	gitlabModels "github.com/merico-dev/lake/plugins/gitlab/models"
	"github.com/merico-dev/lake/plugins/gitlab/tasks"
	"github.com/merico-dev/lake/utils"
	"github.com/mitchellh/mapstructure"
)

var _ core.Plugin = (*Gitlab)(nil)

type GitlabOptions struct {
	Tasks []string `json:"tasks,omitempty"`
}
type Gitlab string

func (plugin Gitlab) Description() string {
	return "To collect and enrich data from Gitlab"
}

func (plugin Gitlab) Init() {
	logger.Info("INFO >>> init go plugin", true)
	err := lakeModels.Db.AutoMigrate(
		&models.GitlabProject{},
		&models.GitlabMergeRequest{},
		&models.GitlabCommit{},
		&models.GitlabProjectCommit{},
		&models.GitlabPipeline{},
		&models.GitlabReviewer{},
		&models.GitlabMergeRequestNote{},
		&models.GitlabMergeRequestCommit{},
		&models.GitlabMergeRequestCommitMergeRequest{},
		&models.GitlabUser{},
	)
	if err != nil {
		logger.Error("Error migrating gitlab: ", err)
		panic(err)
	}
}

func (plugin Gitlab) Execute(options map[string]interface{}, progress chan<- float32, ctx context.Context) error {
	logger.Print("start gitlab plugin execution")

	rateLimitPerSecondInt, err := core.GetRateLimitPerSecond(options, 15)
	if err != nil {
		return err
	}

	// GitLab's authenticated api rate limit is 2000 per min
	// 30 tasks/min 60s/min = 1800 per min < 2000 per min
	// You would think this would work but it hits the rate limit every time. I have to play with the number to see the right way to set it
	scheduler, err := utils.NewWorkerScheduler(50, rateLimitPerSecondInt, ctx)
	defer scheduler.Release()
	if err != nil {
		return fmt.Errorf("could not create scheduler")
	}

	projectId, ok := options["projectId"]
	if !ok {
		return fmt.Errorf("projectId is required for gitlab execution")
	}

	projectIdInt := int(projectId.(float64))
	if projectIdInt < 0 {
		return fmt.Errorf("projectId is invalid")
	}

	var op GitlabOptions
	err = mapstructure.Decode(options, &op)
	if err != nil {
		return err
	}
	tasksToRun := make(map[string]bool, len(op.Tasks))
	for _, task := range op.Tasks {
		tasksToRun[task] = true
	}
	if len(tasksToRun) == 0 {
		tasksToRun = map[string]bool{
			"collectPipelines": true,
			"collectCommits":   true,
			"collectMrs":       true,
			"enrichMrs":        true,
			"convertProjects":  true,
			"convertMrs":       true,
			"convertCommits":   true,
			"convertNotes":     true,
		}
	}
	progress <- 0.1
	if err := tasks.CollectProject(projectIdInt); err != nil {
		return fmt.Errorf("could not collect projects: %v", err)
	}
	if tasksToRun["collectCommits"] {
		progress <- 0.25
		if err := tasks.CollectCommits(projectIdInt, scheduler); err != nil {
			return &errors.SubTaskError{
				SubTaskName: "collectCommits",
				Message:     fmt.Errorf("could not collect commits: %v", err).Error(),
			}
		}
	}
	if tasksToRun["collectMrs"] {
		progress <- 0.3
		mergeRequestErr := tasks.CollectMergeRequests(projectIdInt, scheduler)
		if mergeRequestErr != nil {
			return &errors.SubTaskError{
				SubTaskName: "collectMrs",
				Message:     fmt.Errorf("could not collect merge requests: %v", mergeRequestErr).Error(),
			}
		}
		progress <- 0.4
		collectChildrenOnMergeRequests(projectIdInt, scheduler)
	}
	if tasksToRun["enrichMrs"] {
		progress <- 0.5
		enrichErr := tasks.EnrichMergeRequests()
		if enrichErr != nil {
			return &errors.SubTaskError{
				SubTaskName: "enrichMrs",
				Message:     fmt.Errorf("could not enrich merge requests: %v", enrichErr).Error(),
			}
		}
	}
	if tasksToRun["collectPipelines"] {
		progress <- 0.6
		if err := tasks.CollectAllPipelines(projectIdInt, scheduler); err != nil {
			return &errors.SubTaskError{
				SubTaskName: "collectPipelines",
				Message:     fmt.Errorf("could not collect pipelines: %v", err).Error(),
			}
		}
		if err := tasks.CollectChildrenOnPipelines(projectIdInt, scheduler); err != nil {
			return &errors.SubTaskError{
				SubTaskName: "collectChildrenOnPipelines",
				Message:     fmt.Errorf("could not collect children pipelines: %v", err).Error(),
			}
		}
	}
	if tasksToRun["convertProjects"] {
		progress <- 0.7
		err = tasks.ConvertProjects()
		if err != nil {
			return &errors.SubTaskError{
				SubTaskName: "convertProjects",
				Message:     err.Error(),
			}
		}
	}
	if tasksToRun["convertMrs"] {
		progress <- 0.75
		err = tasks.ConvertPrs()
		if err != nil {
			return &errors.SubTaskError{
				SubTaskName: "convertMrs",
				Message:     err.Error(),
			}
		}
	}
	if tasksToRun["convertCommits"] {
		progress <- 0.8
		err = tasks.ConvertCommits(projectIdInt)
		if err != nil {
			return &errors.SubTaskError{
				SubTaskName: "convertCommits",
				Message:     err.Error(),
			}
		}
	}
	if tasksToRun["convertNotes"] {
		progress <- 0.9
		err = tasks.ConvertNotes()
		if err != nil {
			return &errors.SubTaskError{
				SubTaskName: "convertNotes",
				Message:     err.Error(),
			}
		}
	}
	progress <- 1
	return nil
}

func collectNotesWithScheduler(projectIdInt int, scheduler *utils.WorkerScheduler, mrs []gitlabModels.GitlabMergeRequest) {
	for i := 0; i < len(mrs); i++ {
		mr := (mrs)[i]
		err := scheduler.Submit(func() error {
			notesErr := tasks.CollectMergeRequestNotes(projectIdInt, &mr)
			if notesErr != nil {
				logger.Error("Could not collect MR Notes", notesErr)
				return notesErr
			}
			return nil
		})
		if err != nil {
			logger.Error("err", err)
			return
		}
	}

	scheduler.WaitUntilFinish()
}
func collectCommitsWithScheduler(projectIdInt int, scheduler *utils.WorkerScheduler, mrs []gitlabModels.GitlabMergeRequest) {
	for i := 0; i < len(mrs); i++ {
		mr := (mrs)[i]
		err := scheduler.Submit(func() error {
			commitsErr := tasks.CollectMergeRequestCommits(projectIdInt, &mr)
			if commitsErr != nil {
				logger.Error("Could not collect MR Commits", commitsErr)
				return commitsErr
			}
			return nil
		})
		if err != nil {
			logger.Error("err", err)
			return
		}
	}

	scheduler.WaitUntilFinish()
}

func collectChildrenOnMergeRequests(projectIdInt int, scheduler *utils.WorkerScheduler) {
	// find all mrs from db
	var mrs []gitlabModels.GitlabMergeRequest
	lakeModels.Db.Find(&mrs)

	collectNotesWithScheduler(projectIdInt, scheduler, mrs)
	collectCommitsWithScheduler(projectIdInt, scheduler, mrs)
}

func (plugin Gitlab) RootPkgPath() string {
	return "github.com/merico-dev/lake/plugins/gitlab"
}

func (plugin Gitlab) ApiResources() map[string]map[string]core.ApiResourceHandler {
	return map[string]map[string]core.ApiResourceHandler{
		"test": {
			"POST": api.TestConnection,
		},
		"sources": {
			"GET":  api.ListSources,
			"POST": api.PutSource,
		},
		"sources/:sourceId": {
			"GET": api.GetSource,
			"PUT": api.PutSource,
		},
	}
}

// Export a variable named PluginEntry for Framework to search and load
var PluginEntry Gitlab //nolint

// standalone mode for debugging
func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		panic(fmt.Errorf("usage: go run ./plugins/gitlab <project_id>"))
	}
	projectId, err := strconv.ParseFloat(args[0], 64)
	if err != nil {
		panic(fmt.Errorf("error paring board_id: %w", err))
	}

	err = core.RegisterPlugin("gitlab", PluginEntry)
	if err != nil {
		panic(err)
	}
	PluginEntry.Init()
	progress := make(chan float32)
	go func() {
		err2 := PluginEntry.Execute(
			map[string]interface{}{
				"projectId": projectId,
				//"tasks":     []string{"collectProject"},
				//"tasks":     []string{"collectCommits"},
				"tasks": []string{"convertProjects"},
				//"tasks": []string{"convertCommits"},
			},
			progress,
			context.Background(),
		)
		if err2 != nil {
			panic(err2)
		}
		close(progress)
	}()
	for p := range progress {
		fmt.Println(p)
	}
}
