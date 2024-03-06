package adoclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/microsoft/azure-devops-go-api/azuredevops"
)

type Client struct {
	Creds *azidentity.DefaultAzureCredential
	// Organization is the name of the Azure DevOps organization.
	Organization string
}

func (c *Client) prepareRequest(ctx context.Context, req *http.Request) error {
	token, err := c.Creds.GetToken(ctx, policy.TokenRequestOptions{
		// Scope ID for Azure DevOps.
		// This was hardcoded and copied from the Go Azure DevOps SDK.
		Scopes: []string{"499b84ac-1321-427f-aa17-267ca6975798/.default"},
	})
	if err != nil {
		return fmt.Errorf("getting token: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token.Token)
	req.Header.Add("Accept", "application/json")
	// Set default content type if not set, and there is a body.
	if req.Header.Get("Content-Type") == "" && req.Body != nil {
		req.Header.Add("Content-Type", "application/json;charset=utf-8")
	}

	values := req.URL.Query()
	values.Add("api-version", "7.1-preview.4")
	req.URL.RawQuery = values.Encode()

	return nil
}

// handleRespError takes an http response from the Azure DevOps REST API and if
// the status code is non-2xx it decodes the response body into a WrappedError.
// If the status code is 2xx it returns nil.
func (c *Client) handleRespError(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}

	if response.ContentLength == 0 {
		message := "response with 0 content length returned status: " + response.Status
		return &APIError{
			Message:    message,
			StatusCode: response.StatusCode,
		}
	}

	if response.Header.Get("Content-Type") == "text/plain" {
		buf := strings.Builder{}
		if _, err := io.Copy(&buf, response.Body); err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}
		return &APIError{
			Message:    buf.String(),
			StatusCode: response.StatusCode,
		}
	}

	// The ADO REST API is wonderful and might not return a proper error even in
	// case of an error.
	// Hence, the Go Azure DevOps SDK also has a WrappedImproperError type that
	// should be used if the error message in WrappedError is nil.
	//
	// As a result, we need to store the body in a buffer because we might need
	// to read the body twice.
	//
	// This is all based on reading the code:
	// https://github.com/microsoft/azure-devops-go-api/blob/c9e5fa06da2c96efdb063352524aa458e7733bb6/azuredevops/v7/client.go#L408
	secondBuf := bytes.Buffer{}
	// Read body into buf. When buf is read, it will write to backupBuf, and we
	// use that for the subsequent decoding.
	buf := io.TeeReader(response.Body, &secondBuf)

	var wrappedError azuredevops.WrappedError
	if err := json.NewDecoder(buf).Decode(&wrappedError); err != nil {
		return fmt.Errorf("decoding response into wrapped error: %w", err)
	}
	if wrappedError.Message == nil {
		// If the message in wrapped error is nil, that means we probably/maybe
		// have an improper error, so decode our second buffer into an improper
		// error.
		var improperError azuredevops.WrappedImproperError
		if err := json.NewDecoder(&secondBuf).Decode(&improperError); err != nil {
			return fmt.Errorf("decoding response into improper error: %w", err)
		}
		return &APIError{
			Message:    *improperError.Value.Message,
			StatusCode: response.StatusCode,
		}
	}

	return &APIError{
		Message:    *wrappedError.Message,
		StatusCode: response.StatusCode,
	}
}

func (c *Client) organizationURL() (*url.URL, error) {
	orgURL, err := url.Parse("https://dev.azure.com/")
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}
	if c.Organization == "" {
		return nil, fmt.Errorf("organization name is empty")
	}
	orgURL = orgURL.JoinPath(c.Organization)
	return orgURL, nil
}

var _ error = (*APIError)(nil)

type APIError struct {
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf(
		"%s (status %d - %s)",
		e.Message,
		e.StatusCode,
		http.StatusText(e.StatusCode),
	)
}
