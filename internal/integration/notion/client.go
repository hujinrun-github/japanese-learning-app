package notion

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAPIBaseURL = "https://api.notion.com"
	NotionVersion     = "2025-09-03"
)

// Page is the subset of a Notion page needed by note synchronization.
type Page struct {
	ID             string                  `json:"id"`
	URL            string                  `json:"url"`
	LastEditedTime string                  `json:"last_edited_time"`
	Archived       bool                    `json:"archived,omitempty"`
	InTrash        bool                    `json:"in_trash,omitempty"`
	Properties     map[string]PageProperty `json:"properties,omitempty"`
}

type PageProperty struct {
	ID          string         `json:"id,omitempty"`
	Type        string         `json:"type,omitempty"`
	Title       []RichText     `json:"title,omitempty"`
	RichText    []RichText     `json:"rich_text,omitempty"`
	MultiSelect []SelectOption `json:"multi_select,omitempty"`
}

type RichText struct {
	PlainText string `json:"plain_text,omitempty"`
	Text      *struct {
		Content string `json:"content,omitempty"`
	} `json:"text,omitempty"`
}

type SelectOption struct {
	Name string `json:"name"`
}

// Block is an unmodified Notion block payload. Keeping it generic allows the
// client to preserve the API shape while the converter controls supported types.
type Block map[string]any

type PageInput struct {
	DataSourceID   string
	TitleProperty  string
	TagsProperty   string
	FlowIDProperty string
	Title          string
	Tags           []string
	FlowID         string
	Children       []Block
}

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	retrySleep func(time.Duration)
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("notion API error %d", e.StatusCode)
	}
	return fmt.Sprintf("notion API error %d: %s", e.StatusCode, e.Message)
}

// NewClient creates a client. Use NewClientChecked when the token may be empty.
func NewClient(token, baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	return &Client{
		token:      strings.TrimSpace(token),
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		retrySleep: time.Sleep,
	}
}

func NewClientChecked(token, baseURL string) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("notion token is required")
	}
	return NewClient(token, baseURL), nil
}

func (c *Client) QueryDataSource(dataSourceID string) ([]Page, error) {
	dataSourceID = NormalizeDataSourceID(dataSourceID)
	if dataSourceID == "" {
		return nil, errors.New("notion data source id is required")
	}
	var pages []Page
	cursor := ""
	for {
		body := map[string]any{}
		if cursor != "" {
			body["start_cursor"] = cursor
		}
		var response struct {
			Results    []Page `json:"results"`
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
		}
		path := "/v1/data_sources/" + url.PathEscape(dataSourceID) + "/query"
		if err := c.doJSON(http.MethodPost, path, body, &response); err != nil {
			return nil, fmt.Errorf("notion query data source: %w", err)
		}
		pages = append(pages, response.Results...)
		if !response.HasMore {
			return pages, nil
		}
		cursor = strings.TrimSpace(response.NextCursor)
		if cursor == "" {
			return nil, errors.New("notion query returned has_more without next_cursor")
		}
	}
}

func (c *Client) TestDataSource(dataSourceID string) error {
	_, err := c.QueryDataSource(dataSourceID)
	return err
}

func (c *Client) RetrievePageBlocks(pageID string) ([]Block, error) {
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return nil, errors.New("notion page id is required")
	}
	var blocks []Block
	cursor := ""
	for {
		path := "/v1/blocks/" + url.PathEscape(pageID) + "/children"
		if cursor != "" {
			path += "?start_cursor=" + url.QueryEscape(cursor)
		}
		var response struct {
			Results    []Block `json:"results"`
			HasMore    bool    `json:"has_more"`
			NextCursor string  `json:"next_cursor"`
		}
		if err := c.doJSON(http.MethodGet, path, nil, &response); err != nil {
			return nil, fmt.Errorf("notion retrieve blocks: %w", err)
		}
		blocks = append(blocks, response.Results...)
		if !response.HasMore {
			return blocks, nil
		}
		cursor = strings.TrimSpace(response.NextCursor)
		if cursor == "" {
			return nil, errors.New("notion blocks returned has_more without next_cursor")
		}
	}
}

func (c *Client) CreatePage(input PageInput) (Page, error) {
	var page Page
	input.DataSourceID = NormalizeDataSourceID(input.DataSourceID)
	if input.DataSourceID == "" {
		return page, errors.New("notion data source id is required")
	}
	body := map[string]any{
		"parent":     map[string]any{"type": "data_source_id", "data_source_id": input.DataSourceID},
		"properties": propertyPayload(input),
	}
	if len(input.Children) > 0 {
		body["children"] = input.Children
	}
	if err := c.doJSON(http.MethodPost, "/v1/pages", body, &page); err != nil {
		return page, fmt.Errorf("notion create page: %w", err)
	}
	return page, nil
}

func (c *Client) UpdatePage(pageID string, input PageInput) (Page, error) {
	var page Page
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return page, errors.New("notion page id is required")
	}
	body := map[string]any{"properties": propertyPayload(input)}
	if err := c.doJSON(http.MethodPatch, "/v1/pages/"+url.PathEscape(pageID), body, &page); err != nil {
		return page, fmt.Errorf("notion update page: %w", err)
	}
	return page, nil
}

func (c *Client) RestorePage(pageID string) error {
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return errors.New("notion page id is required")
	}
	if err := c.doJSON(http.MethodPatch, "/v1/pages/"+url.PathEscape(pageID), map[string]any{"archived": false}, nil); err != nil {
		return fmt.Errorf("notion restore page: %w", err)
	}
	return nil
}

func (c *Client) ArchiveBlock(blockID string) error {
	if strings.TrimSpace(blockID) == "" {
		return errors.New("notion block id is required")
	}
	if err := c.doJSON(http.MethodPatch, "/v1/blocks/"+url.PathEscape(blockID), map[string]any{"archived": true}, nil); err != nil {
		return fmt.Errorf("notion archive block: %w", err)
	}
	return nil
}

func (c *Client) AppendBlockChildren(parentID string, children []Block) error {
	if strings.TrimSpace(parentID) == "" {
		return errors.New("notion parent block id is required")
	}
	if len(children) == 0 {
		return nil
	}
	if err := c.doJSON(http.MethodPost, "/v1/blocks/"+url.PathEscape(parentID)+"/children", map[string]any{"children": children}, nil); err != nil {
		return fmt.Errorf("notion append blocks: %w", err)
	}
	return nil
}

// ReplacePageBlocks archives the current direct children and appends the new
// supported block set. Notion pages do not expose an atomic children replace
// endpoint, so callers should use this only for content that this integration
// owns.
func (c *Client) ReplacePageBlocks(pageID string, children []Block) error {
	existing, err := c.RetrievePageBlocks(pageID)
	if err != nil {
		return fmt.Errorf("notion replace blocks list: %w", err)
	}
	for _, block := range existing {
		id, _ := block["id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		if err := c.ArchiveBlock(id); err != nil {
			return fmt.Errorf("notion replace blocks archive: %w", err)
		}
	}
	if err := c.AppendBlockChildren(pageID, children); err != nil {
		return fmt.Errorf("notion replace blocks append: %w", err)
	}
	return nil
}

func (c *Client) doJSON(method, path string, body, output any) error {
	for attempt := 0; attempt < 3; attempt++ {
		var requestBody io.Reader = http.NoBody
		if body != nil {
			payload, err := encodeJSON(body)
			if err != nil {
				return err
			}
			requestBody = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, c.baseURL+path, requestBody)
		if err != nil {
			return fmt.Errorf("notion create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Notion-Version", NotionVersion)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("notion request: %w", err)
		}
		err = decodeResponse(resp, output)
		status := resp.StatusCode
		if err == nil {
			return nil
		}
		if !retryable(status) || attempt == 2 {
			return err
		}
		delay := time.Duration(attempt+1) * 250 * time.Millisecond
		if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
			if seconds, parseErr := strconv.Atoi(raw); parseErr == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			}
		}
		c.retrySleep(delay)
	}
	return errors.New("notion request exhausted retries")
}

func decodeResponse(resp *http.Response, output any) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("notion read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &payload)
		return APIError{StatusCode: resp.StatusCode, Message: payload.Message}
	}
	if output == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("notion decode response: %w", err)
	}
	return nil
}

func retryable(status int) bool { return status == http.StatusTooManyRequests || status >= 500 }

func propertyPayload(input PageInput) map[string]any {
	titleName := input.TitleProperty
	if strings.TrimSpace(titleName) == "" {
		titleName = "Name"
	}
	payload := map[string]any{
		titleName: map[string]any{"title": []map[string]any{{"text": map[string]any{"content": input.Title}}}},
	}
	if tagsName := strings.TrimSpace(input.TagsProperty); tagsName != "" {
		options := make([]map[string]any, 0, len(input.Tags))
		for _, tag := range normalizeTags(input.Tags) {
			options = append(options, map[string]any{"name": tag})
		}
		payload[tagsName] = map[string]any{"multi_select": options}
	}
	if flowIDName := strings.TrimSpace(input.FlowIDProperty); flowIDName != "" && strings.TrimSpace(input.FlowID) != "" {
		payload[flowIDName] = map[string]any{"rich_text": []map[string]any{{"text": map[string]any{"content": input.FlowID}}}}
	}
	return payload
}
