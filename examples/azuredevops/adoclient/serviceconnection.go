package adoclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/serviceendpoint"
	"github.com/verifa/horizon/pkg/hz"
)

type GetServiceConnectionRequest struct {
	// ProjectID is the project in which the service connection exists.
	ProjectID string
	// ID is the ID of the service connection to get.
	ID string
}

type GetServiceConnectionResponse struct {
	ServiceEndpoint *serviceendpoint.ServiceEndpoint
}

func (c *Client) GetServiceConnection(
	ctx context.Context,
	req GetServiceConnectionRequest,
) (*GetServiceConnectionResponse, error) {
	reqURL, err := c.organizationURL()
	if err != nil {
		return nil, fmt.Errorf("getting organization URL: %w", err)
	}
	// GET
	// https://dev.azure.com/{organization}/{project}/_apis/serviceendpoint/endpoints/{endpointId}?api-version=7.1-preview.4
	reqURL = reqURL.JoinPath(
		req.ProjectID,
		"_apis",
		"serviceendpoint",
		"endpoints",
		req.ID,
	)

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

	var result serviceendpoint.ServiceEndpoint
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response body: %w", err)
	}
	return &GetServiceConnectionResponse{
		ServiceEndpoint: &result,
	}, nil
}

type ListServiceConnectionRequest struct {
	// ProjectID is the project in which the service connection exists.
	ProjectID string
	// Name is the name of the service connection to get.
	Name string
}

type ListServiceConnectionResponse struct {
	// ID is the ID of the service connection.
	ID uuid.UUID
}

func (c *Client) ListServiceConnection(
	ctx context.Context,
	req ListServiceConnectionRequest,
) (*ListServiceConnectionResponse, error) {
	reqURL, err := c.organizationURL()
	if err != nil {
		return nil, fmt.Errorf("getting organization URL: %w", err)
	}
	// GET
	// https://dev.azure.com/{organization}/{project}/_apis/serviceendpoint/endpoints?endpointNames={endpointNames}&api-version=7.1-preview.4
	reqURL = reqURL.JoinPath(
		req.ProjectID,
		"_apis",
		"serviceendpoint",
		"endpoints",
	)
	values := reqURL.Query()
	values.Add("endpointNames", req.Name)
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

	type Result struct {
		Count int                               `json:"count"`
		Value []serviceendpoint.ServiceEndpoint `json:"value"`
	}
	var result Result
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response body: %w", err)
	}
	if len(result.Value) != 1 {
		return nil, fmt.Errorf(
			"expected 1 service connection, got %d",
			len(result.Value),
		)
	}
	return &ListServiceConnectionResponse{
		ID: *result.Value[0].Id,
	}, nil
}

type CreateServiceConnectionRequest struct {
	Name             string
	SubscriptionName string
	SubscriptionID   string
	// ProjectID is the project in which the service connection exists.
	ProjectID uuid.UUID

	// ServicePrincipalID is the Object ID of the app registration.
	ServicePrincipalID string
	// TenantID is the tenant ID of the app registration.
	TenantID string
}

type CreateServiceConnectionResponse struct {
	ServiceEndpoint *serviceendpoint.ServiceEndpoint
}

func (c *Client) CreateServiceConnection(
	ctx context.Context,
	req CreateServiceConnectionRequest,
) (*CreateServiceConnectionResponse, error) {
	reqURL, err := c.organizationURL()
	if err != nil {
		return nil, fmt.Errorf("getting organization URL: %w", err)
	}
	// POST
	// https://dev.azure.com/{organization}/_apis/serviceendpoint/endpoints?api-version=7.1-preview.4
	reqURL = reqURL.JoinPath(
		"_apis",
		"serviceendpoint",
		"endpoints",
	)

	svcEndpoint := serviceendpoint.ServiceEndpoint{
		Authorization: &serviceendpoint.EndpointAuthorization{
			Parameters: &map[string]string{
				// This is the Azure Entra ID App Registration to use.
				// For now, hard code the values from app reg:
				// verifa-hz-horizon-12749df0-9a8e-44cd-889e-4740be851c13
				"serviceprincipalid": req.ServicePrincipalID,
				"tenantid":           req.TenantID,
			},
			Scheme: hz.P("WorkloadIdentityFederation"),
		},
		Data: &map[string]string{
			"creationMode":   "Manual",
			"environment":    "AzureCloud",
			"scopeLevel":     "Subscription",
			"subscriptionId": req.SubscriptionID,
			// This is needed, not sure why when we have the ID, but it fails
			// without it.
			"subscriptionName": req.SubscriptionName,
		},
		Description: hz.P("TODO: created by Horizon"),
		Name:        &req.Name,
		ServiceEndpointProjectReferences: &[]serviceendpoint.ServiceEndpointProjectReference{
			{
				// Project details.
				Name: &req.Name,
				ProjectReference: &serviceendpoint.ProjectReference{
					Id: &req.ProjectID,
				},
			},
		},
		Type: hz.P("azurerm"),
		Url:  hz.P("https://management.azure.com/"),
	}

	reqBody := bytes.Buffer{}
	if err := json.NewEncoder(&reqBody).Encode(svcEndpoint); err != nil {
		return nil, fmt.Errorf("encoding body: %w", err)
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

	var result serviceendpoint.ServiceEndpoint
	if err := json.NewDecoder(httpResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response body: %w", err)
	}
	return &CreateServiceConnectionResponse{
		ServiceEndpoint: &result,
	}, nil
}
