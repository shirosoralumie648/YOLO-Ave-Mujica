package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type APIArtifactSource struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewAPIArtifactSource(baseURL string) *APIArtifactSource {
	return &APIArtifactSource{BaseURL: baseURL}
}

func (s *APIArtifactSource) FetchArtifact(format, version string) (PulledArtifact, error) {
	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resolveURL := fmt.Sprintf("%s/v1/artifacts/resolve?format=%s&version=%s",
		strings.TrimRight(s.BaseURL, "/"),
		url.QueryEscape(format),
		url.QueryEscape(version),
	)

	var artifact struct {
		ID      int64  `json:"id"`
		Version string `json:"version"`
	}
	if err := fetchJSON(client, resolveURL, &artifact); err != nil {
		return PulledArtifact{}, err
	}

	return PulledArtifact{
		ArtifactID: artifact.ID,
		Version:    artifact.Version,
		Entries:    []ArtifactEntry{},
	}, nil
}

func fetchJSON(client *http.Client, targetURL string, out any) error {
	resp, err := client.Get(targetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d for %s: %s", resp.StatusCode, targetURL, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
