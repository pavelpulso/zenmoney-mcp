package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nemirlev/zenmoney-go-sdk/v2/api"
	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

// diffURL is the Zenmoney sync endpoint. PushTags posts here directly instead of
// going through the SDK - see PushTags for why.
const diffURL = "https://api.zenmoney.ru/v8/diff/"

// ZenClient is the interface that all tool handlers depend on.
// The production implementation wraps the ZenMoney Go SDK; test code uses a mock.
type ZenClient interface {
	// FullSync fetches all data from ZenMoney, ignoring any cached sync state.
	FullSync(ctx context.Context) (models.Response, error)

	// SyncSince fetches only changes since the given timestamp (incremental sync).
	SyncSince(ctx context.Context, since time.Time) (models.Response, error)

	// Sync sends an explicit diff request.
	Sync(ctx context.Context, req models.Request) (models.Response, error)

	// Push sends new/updated/deleted entities to ZenMoney and receives the server's
	// response (which may include additional server-side changes). This is used for all
	// write operations: create, update, and delete.
	Push(ctx context.Context, req models.Request) (models.Response, error)

	PushTags(ctx context.Context, req models.Request) (models.Response, error)

	// Suggest asks ZenMoney to suggest a category (tags, merchant) for a partial transaction.
	Suggest(ctx context.Context, tx models.Transaction) (models.Transaction, error)
}

// Client is the production ZenClient backed by the ZenMoney Go SDK.
type Client struct {
	sdk   *api.Client
	token string
	http  *http.Client
	// pushTagsURL overrides the endpoint in tests.
	pushTagsURL string
}

// New creates a ZenClient authenticated with the given token.
// token is typically read from the ZENMONEY_TOKEN environment variable by the caller.
func New(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("ZENMONEY_TOKEN is not set")
	}
	sdkClient, err := api.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("create zenmoney client: %w", err)
	}
	return &Client{sdk: sdkClient, token: token, http: &http.Client{Timeout: 5 * time.Minute}}, nil
}

func (c *Client) FullSync(ctx context.Context) (models.Response, error) {
	return c.sdk.FullSync(ctx)
}

func (c *Client) SyncSince(ctx context.Context, since time.Time) (models.Response, error) {
	return c.sdk.SyncSince(ctx, since)
}

func (c *Client) Sync(ctx context.Context, req models.Request) (models.Response, error) {
	return c.sdk.Sync(ctx, req)
}

func (c *Client) Push(ctx context.Context, req models.Request) (models.Response, error) {
	return c.Sync(ctx, req)
}

// PushTags saves categories. It bypasses the SDK because models.Tag types
// staticId as a plain string, so the SDK always sends "staticId": "" and
// Zenmoney rejects the whole request with 400 `Invalid property "staticId" in
// object Tag ... Wrong value`. A user-created category must send null there, so
// this marshals the request, nulls out empty staticId values, and posts it.
func (c *Client) PushTags(ctx context.Context, req models.Request) (models.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return models.Response{}, fmt.Errorf("marshal tag request: %w", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return models.Response{}, fmt.Errorf("normalize tag request: %w", err)
	}
	if tags, ok := envelope["tag"].([]any); ok {
		for _, raw := range tags {
			tag, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := tag["staticId"].(string); ok && id == "" {
				tag["staticId"] = nil
			}
		}
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return models.Response{}, fmt.Errorf("marshal tag request: %w", err)
	}

	url := diffURL
	if c.pushTagsURL != "" {
		url = c.pushTagsURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return models.Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return models.Response{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return models.Response{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return models.Response{}, fmt.Errorf("zenmoney returned %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}

	var out models.Response
	if err := json.Unmarshal(data, &out); err != nil {
		return models.Response{}, fmt.Errorf("decode tag response: %w", err)
	}
	return out, nil
}

func (c *Client) Suggest(ctx context.Context, tx models.Transaction) (models.Transaction, error) {
	return c.sdk.Suggest(ctx, tx)
}
