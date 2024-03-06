package adoclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/verifa/horizon/pkg/hz"
)

type ProjectWorkItemProcess string

// These are some crazy hardcoded values that do not exist in the Go SDK
// library. The initial source came from StackOverflow and then got the rest
// from existing projects and the REST API.
//
//	https://stackoverflow.com/a/34190267
const (
	ProjectWorkItemProcessBasic ProjectWorkItemProcess = "b8a3a935-7e91-48b8-a94c-606d37c3e9f2"
	ProjectWorkItemProcessAgile ProjectWorkItemProcess = "adcc42ab-9882-485e-a3ed-7678f01f66bc"
	ProjectWorkItemProcessScrum ProjectWorkItemProcess = "6b724908-ef14-45cf-84f8-768b5384da45"
	ProjectWorkItemProcessCMMI  ProjectWorkItemProcess = "27450541-8e31-4150-9947-dc59f998fc01"
)

type ListProjectsRequest struct{}

type ListProjectsResponse struct {
	Projects []core.WebApiProject
}

func (c *Client) ListProjects(
	ctx context.Context,
	req ListProjectsRequest,
) (*ListProjectsResponse, error) {
	reqURL, err := c.organizationURL()
	if err != nil {
		return nil, fmt.Errorf("getting organization URL: %w", err)
	}
	reqURL = reqURL.JoinPath("_apis", "projects")

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		reqURL.String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	if err := c.prepareRequest(ctx, httpReq); err != nil {
		return nil, fmt.Errorf("preparing request: %w", err)
	}

	httpClient := http.Client{
		Timeout: time.Second * 30,
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("performing http request: %w", err)
	}
	defer httpResp.Body.Close()

	if err := c.handleRespError(httpResp); err != nil {
		return nil, err
	}

	type Result struct {
		Count int                  `json:"count"`
		Value []core.WebApiProject `json:"value"`
	}
	var result Result
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &ListProjectsResponse{
		Projects: result.Value,
	}, nil
}

type CreateProjectRequest struct {
	Name string
}

type CreateProjectResponse struct {
	ID *uuid.UUID
}

func (c *Client) CreateProject(
	ctx context.Context,
	req CreateProjectRequest,
) (*CreateProjectResponse, error) {
	reqURL, err := c.organizationURL()
	if err != nil {
		return nil, fmt.Errorf("getting organization URL: %w", err)
	}
	reqURL = reqURL.JoinPath("_apis", "projects")

	project := core.WebApiProject{
		Name: &req.Name,

		// Required values based on errors from the API:
		//
		// 	The project information supplied to project create is invalid. You
		// 	must provide all and only these properties/capabilities: name,
		// 	description, visibility,
		// 	capabilities.versioncontrol.sourceControlType,
		// 	capabilities.processTemplate.templateTypeId. (status 400 - Bad
		// 	Request)
		Description: hz.P("TODO: created by horizon"),
		Visibility:  &core.ProjectVisibilityValues.Private,
		Capabilities: &map[string]map[string]string{
			"versioncontrol": {
				"sourceControlType": string(core.SourceControlTypesValues.Git),
			},
			"processTemplate": {
				"templateTypeId": string(ProjectWorkItemProcessBasic),
			},
		},
	}

	reqBody := bytes.Buffer{}
	if err := json.NewEncoder(&reqBody).Encode(project); err != nil {
		return nil, fmt.Errorf("encoding request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		reqURL.String(),
		&reqBody,
	)
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	if err := c.prepareRequest(ctx, httpReq); err != nil {
		return nil, fmt.Errorf("preparing request: %w", err)
	}

	fmt.Println(httpReq.URL.String())

	httpClient := http.Client{
		Timeout: time.Second * 30,
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("performing http request: %w", err)
	}
	defer httpResp.Body.Close()

	if err := c.handleRespError(httpResp); err != nil {
		return nil, err
	}

	// This is an example response body from the Azure DevOps REST API:
	// 	{"id":"c2c65c2a-7770-4013-973e-25c33287c464","status":"notSet","url":"https://dev.azure.com/verifa-hz/_apis/operations/c2c65c2a-7770-4013-973e-25c33287c464"}

	type Result struct {
		ID *uuid.UUID `json:"id"`
	}
	var result Result
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &CreateProjectResponse{
		ID: result.ID,
	}, nil
}

type DeleteProjectRequest struct {
	ID uuid.UUID
}

func (c *Client) DeleteProject(
	ctx context.Context,
	req DeleteProjectRequest,
) error {
	reqURL, err := c.organizationURL()
	if err != nil {
		return fmt.Errorf("getting organization URL: %w", err)
	}

	reqURL = reqURL.JoinPath("_apis", "projects", req.ID.String())

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		reqURL.String(),
		nil,
	)
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}
	if err := c.prepareRequest(ctx, httpReq); err != nil {
		return fmt.Errorf("preparing request: %w", err)
	}
	httpClient := http.Client{
		Timeout: time.Second * 30,
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("performing http request: %w", err)
	}
	defer httpResp.Body.Close()
	return c.handleRespError(httpResp)
}
