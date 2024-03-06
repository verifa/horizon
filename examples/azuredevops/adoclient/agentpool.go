package adoclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/elastic"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/taskagent"
	"github.com/verifa/horizon/pkg/hz"
)

type ListAgentPoolsRequest struct {
	FilterName string
}

type ListAgentPoolsResponse struct {
	AgentPools []taskagent.TaskAgentPool
}

func (c *Client) ListAgentPools(
	ctx context.Context,
	req ListAgentPoolsRequest,
) (*ListAgentPoolsResponse, error) {
	reqURL, err := c.organizationURL()
	if err != nil {
		return nil, fmt.Errorf("getting organization URL: %w", err)
	}
	reqURL = reqURL.JoinPath(
		"_apis",
		"distributedtask",
		"pools",
	)
	values := reqURL.Query()
	if req.FilterName != "" {
		values.Add("poolName", req.FilterName)
	}
	reqURL.RawQuery = values.Encode()

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

	// Taken from the Go Azure DevOps SDK.
	// https://github.com/microsoft/azure-devops-go-api/blob/c9e5fa06da2c96efdb063352524aa458e7733bb6/azuredevops/v7/models.go#L123
	type Result struct {
		Count int                       `json:"count"`
		Value []taskagent.TaskAgentPool `json:"value"`
	}
	var result Result
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &ListAgentPoolsResponse{
		AgentPools: result.Value,
	}, nil
}

type CreateAgentPoolRequest struct {
	// Name is the name of the agent pool to create.
	Name string
	// VMScaleSetID is the resource ID of the virtual machine scale set that the
	// agent pool will use for running pipelines on.
	VMScaleSetID string

	ProjectID string

	// ServiceConnectionID is the resource ID of the service connection.
	// The service connection is used by Azure DevOps to perform operations in
	// Azure.
	ServiceConnectionID string
}

type CreateAgentPoolResponse struct {
	ID   int
	Name string
}

func (c *Client) CreateAgentPool(
	ctx context.Context,
	req CreateAgentPoolRequest,
) (*CreateAgentPoolResponse, error) {
	serviceEndpointScope, err := uuid.Parse(req.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("parsing project ID: %w", err)
	}
	serviceEndpointID, err := uuid.Parse(req.ServiceConnectionID)
	if err != nil {
		return nil, fmt.Errorf("parsing service connection ID: %w", err)
	}
	elasticPool := elastic.ElasticPool{
		// AzureId is the VMScaleSet ID.
		// It is a very misleading name...
		AzureId: &req.VMScaleSetID,
		// ServiceEndpointScope is ID of the Azure DevOps project.
		ServiceEndpointScope: &serviceEndpointScope,
		// ServiceEndpointId is the ID of the service connection.
		ServiceEndpointId: &serviceEndpointID,

		MaxCapacity:         hz.P(2),
		DesiredIdle:         hz.P(0),
		AgentInteractiveUI:  hz.P(false),
		RecycleAfterEachUse: hz.P(false),
		TimeToLiveMinutes:   hz.P(30),
	}

	reqBody := bytes.Buffer{}
	if err := json.NewEncoder(&reqBody).Encode(elasticPool); err != nil {
		return nil, fmt.Errorf("encoding elastic pool: %w", err)
	}

	reqURL, err := c.organizationURL()
	if err != nil {
		return nil, fmt.Errorf("getting organization URL: %w", err)
	}
	reqURL = reqURL.JoinPath(
		"_apis",
		"distributedtask",
		"elasticpools",
	)

	values := reqURL.Query()
	values.Add("poolName", req.Name)
	values.Add("projectId", req.ProjectID)
	reqURL.RawQuery = values.Encode()

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

	var result elastic.ElasticPoolCreationResult
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &CreateAgentPoolResponse{
		ID:   *result.AgentPool.Id,
		Name: *result.AgentPool.Name,
	}, nil
}

type DeleteAgentPoolRequest struct {
	ID int
}

func (c *Client) DeleteAgentPool(
	ctx context.Context,
	req DeleteAgentPoolRequest,
) error {
	reqURL, err := c.organizationURL()
	if err != nil {
		return fmt.Errorf("getting organization URL: %w", err)
	}
	reqURL = reqURL.JoinPath(
		"_apis",
		"distributedtask",
		"pools",
		fmt.Sprint(req.ID),
	)

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
