package notion

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var notionIDPattern = regexp.MustCompile(`(?i)[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

const DefaultTokenEnv = "JLA_NOTION_TOKEN"

// Config contains the non-secret settings required to access a Notion data source.
// The token itself is deliberately resolved from the process environment.
type Config struct {
	DataSourceID   string
	TokenEnv       string
	TitleProperty  string
	TagsProperty   string
	FlowIDProperty string
	SyncTags       []string
}

// NormalizeConfig validates and fills defaults for a Notion target.
func NormalizeConfig(cfg Config) (Config, error) {
	cfg.DataSourceID = NormalizeDataSourceID(cfg.DataSourceID)
	if cfg.DataSourceID == "" {
		return Config{}, errors.New("notion data source id is required")
	}
	if strings.TrimSpace(cfg.TokenEnv) == "" {
		cfg.TokenEnv = DefaultTokenEnv
	}
	if err := ValidateTokenEnv(cfg.TokenEnv); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(cfg.TitleProperty) == "" {
		cfg.TitleProperty = "Name"
	}
	if strings.TrimSpace(cfg.TagsProperty) == "" {
		cfg.TagsProperty = "Tags"
	}
	cfg.FlowIDProperty = strings.TrimSpace(cfg.FlowIDProperty)
	cfg.SyncTags = normalizeTags(cfg.SyncTags)
	return cfg, nil
}

// NormalizeDataSourceID accepts a raw Notion UUID, compact UUID, or URL.
func NormalizeDataSourceID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	searchValue := value
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		searchValue = parsed.Path
	}
	matches := notionIDPattern.FindAllString(searchValue, -1)
	if len(matches) == 0 {
		return value
	}
	return normalizeNotionID(matches[len(matches)-1])
}

func normalizeNotionID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 32 {
		return value
	}
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
}

// ValidateTokenEnv prevents a user-controlled target from reading arbitrary secrets.
func ValidateTokenEnv(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if name == DefaultTokenEnv || (strings.HasPrefix(name, "JLA_") && strings.HasSuffix(name, "_NOTION_TOKEN")) {
		return nil
	}
	return fmt.Errorf("notion token env must be %s or a JLA_*_NOTION_TOKEN variable", DefaultTokenEnv)
}

// ResolveToken returns the configured token without ever logging it.
func ResolveToken(cfg Config, getenv func(string) string) (string, error) {
	normalized, err := NormalizeConfig(cfg)
	if err != nil {
		return "", err
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	token := strings.TrimSpace(getenv(normalized.TokenEnv))
	if token == "" {
		return "", fmt.Errorf("%s is not configured", normalized.TokenEnv)
	}
	return token, nil
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func encodeJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("notion encode JSON: %w", err)
	}
	return b, nil
}
