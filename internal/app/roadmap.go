package app

import (
	"context"
	"strings"
	"time"

	"github.com/meteorsky/agentx/internal/domain"
	"github.com/meteorsky/agentx/internal/id"
)

type RoadmapStageCreateRequest struct {
	ProjectID   string
	Name        string
	Description string
}

type RoadmapStageUpdateRequest struct {
	Name        *string
	Description *string
	Status      *string
}

type RoadmapTaskCreateRequest struct {
	StageID     string
	Title       string
	Description string
}

type RoadmapTaskUpdateRequest struct {
	Title       *string
	Description *string
	Completed   *bool
}

type RoadmapStageWithTasks struct {
	Stage domain.RoadmapStage  `json:"stage"`
	Tasks []domain.RoadmapTask `json:"tasks"`
}

func (a *App) ProjectRoadmap(ctx context.Context, projectID string) ([]RoadmapStageWithTasks, error) {
	stages, err := a.store.RoadmapStages().ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	tasks, err := a.store.RoadmapTasks().ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	tasksByStage := make(map[string][]domain.RoadmapTask)
	for _, t := range tasks {
		tasksByStage[t.StageID] = append(tasksByStage[t.StageID], t)
	}
	result := make([]RoadmapStageWithTasks, len(stages))
	for i, stage := range stages {
		stageTasks := tasksByStage[stage.ID]
		if stageTasks == nil {
			stageTasks = []domain.RoadmapTask{}
		}
		result[i] = RoadmapStageWithTasks{Stage: stage, Tasks: stageTasks}
	}
	return result, nil
}

func (a *App) RoadmapStage(ctx context.Context, stageID string) (domain.RoadmapStage, error) {
	return a.store.RoadmapStages().ByID(ctx, stageID)
}

func (a *App) CreateRoadmapStage(ctx context.Context, req RoadmapStageCreateRequest) (domain.RoadmapStage, error) {
	project, err := a.store.Projects().ByID(ctx, req.ProjectID)
	if err != nil {
		return domain.RoadmapStage{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return domain.RoadmapStage{}, invalidInput("stage name is required")
	}
	maxPos, err := a.store.RoadmapStages().MaxPositionByProject(ctx, project.ID)
	if err != nil {
		return domain.RoadmapStage{}, err
	}
	now := time.Now().UTC()
	stage := domain.RoadmapStage{
		ID:             id.New("rms"),
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		Status:         domain.RoadmapStageStatusActive,
		Position:       maxPos + 1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := a.store.RoadmapStages().Create(ctx, stage); err != nil {
		return domain.RoadmapStage{}, err
	}
	return stage, nil
}

func (a *App) UpdateRoadmapStage(ctx context.Context, stageID string, req RoadmapStageUpdateRequest) (domain.RoadmapStage, error) {
	stage, err := a.store.RoadmapStages().ByID(ctx, stageID)
	if err != nil {
		return domain.RoadmapStage{}, err
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return domain.RoadmapStage{}, invalidInput("stage name is required")
		}
		stage.Name = name
	}
	if req.Description != nil {
		stage.Description = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		status := domain.RoadmapStageStatus(*req.Status)
		if status != domain.RoadmapStageStatusActive && status != domain.RoadmapStageStatusCompleted {
			return domain.RoadmapStage{}, invalidInput("invalid stage status")
		}
		stage.Status = status
	}
	stage.UpdatedAt = time.Now().UTC()
	if err := a.store.RoadmapStages().Update(ctx, stage); err != nil {
		return domain.RoadmapStage{}, err
	}
	return stage, nil
}

func (a *App) DeleteRoadmapStage(ctx context.Context, stageID string) error {
	return a.store.RoadmapStages().Delete(ctx, stageID)
}

func (a *App) ReorderRoadmapStages(ctx context.Context, projectID string, ids []string) error {
	return a.store.RoadmapStages().ReorderByProject(ctx, projectID, ids)
}

func (a *App) RoadmapTask(ctx context.Context, taskID string) (domain.RoadmapTask, error) {
	return a.store.RoadmapTasks().ByID(ctx, taskID)
}

func (a *App) CreateRoadmapTask(ctx context.Context, req RoadmapTaskCreateRequest) (domain.RoadmapTask, error) {
	stage, err := a.store.RoadmapStages().ByID(ctx, req.StageID)
	if err != nil {
		return domain.RoadmapTask{}, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return domain.RoadmapTask{}, invalidInput("task title is required")
	}
	maxPos, err := a.store.RoadmapTasks().MaxPositionByStage(ctx, stage.ID)
	if err != nil {
		return domain.RoadmapTask{}, err
	}
	now := time.Now().UTC()
	task := domain.RoadmapTask{
		ID:             id.New("rmt"),
		OrganizationID: stage.OrganizationID,
		StageID:        stage.ID,
		Title:          title,
		Description:    strings.TrimSpace(req.Description),
		Position:       maxPos + 1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := a.store.RoadmapTasks().Create(ctx, task); err != nil {
		return domain.RoadmapTask{}, err
	}
	return task, nil
}

func (a *App) UpdateRoadmapTask(ctx context.Context, taskID string, req RoadmapTaskUpdateRequest) (domain.RoadmapTask, error) {
	task, err := a.store.RoadmapTasks().ByID(ctx, taskID)
	if err != nil {
		return domain.RoadmapTask{}, err
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return domain.RoadmapTask{}, invalidInput("task title is required")
		}
		task.Title = title
	}
	if req.Description != nil {
		task.Description = strings.TrimSpace(*req.Description)
	}
	if req.Completed != nil {
		task.Completed = *req.Completed
	}
	task.UpdatedAt = time.Now().UTC()
	if err := a.store.RoadmapTasks().Update(ctx, task); err != nil {
		return domain.RoadmapTask{}, err
	}
	return task, nil
}

func (a *App) DeleteRoadmapTask(ctx context.Context, taskID string) error {
	return a.store.RoadmapTasks().Delete(ctx, taskID)
}

func (a *App) ReorderRoadmapTasks(ctx context.Context, stageID string, ids []string) error {
	return a.store.RoadmapTasks().ReorderByStage(ctx, stageID, ids)
}
