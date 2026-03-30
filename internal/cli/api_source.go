package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type APIArtifactSource struct {
	BaseURL         string
	HTTPClient      *http.Client
	PollInterval    time.Duration
	MaxPollAttempts int
}

func NewAPIArtifactSource(baseURL string) *APIArtifactSource {
	return &APIArtifactSource{BaseURL: baseURL}
}

func (s *APIArtifactSource) FetchArtifact(dataset, format, version string) (PulledArtifact, error) {
	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resolveURL := fmt.Sprintf("%s/v1/artifacts/resolve?format=%s&version=%s",
		strings.TrimRight(s.BaseURL, "/"),
		url.QueryEscape(format),
		url.QueryEscape(version),
	)
	if dataset != "" {
		resolveURL += "&dataset=" + url.QueryEscape(dataset)
	}

	var artifact struct {
		ID      int64  `json:"id"`
		Version string `json:"version"`
	}
	if err := fetchJSON(client, resolveURL, &artifact); err != nil {
		return PulledArtifact{}, err
	}

	artifactURL := fmt.Sprintf("%s/v1/artifacts/%d", strings.TrimRight(s.BaseURL, "/"), artifact.ID)
	var detail struct {
		ID      int64           `json:"id"`
		Version string          `json:"version"`
		Status  string          `json:"status"`
		Entries []ArtifactEntry `json:"entries"`
	}

	maxPollAttempts := s.MaxPollAttempts
	if maxPollAttempts <= 0 {
		maxPollAttempts = 10
	}
	pollInterval := s.PollInterval
	if pollInterval <= 0 {
		pollInterval = 10 * time.Millisecond
	}

	for attempt := 0; attempt < maxPollAttempts; attempt++ {
		if err := fetchJSON(client, artifactURL, &detail); err != nil {
			return PulledArtifact{}, err
		}
		if detail.ID == 0 {
			detail.ID = artifact.ID
		}
		if detail.Version == "" {
			detail.Version = artifact.Version
		}
		if len(detail.Entries) > 0 {
			return PulledArtifact{
				ArtifactID: detail.ID,
				Version:    detail.Version,
				Entries:    detail.Entries,
			}, nil
		}
		if detail.Status == "ready" {
			break
		}
		if attempt == maxPollAttempts-1 {
			return PulledArtifact{}, fmt.Errorf("artifact %d is not ready", detail.ID)
		}
		time.Sleep(pollInterval)
	}

	presignURL := fmt.Sprintf("%s/v1/artifacts/%d/presign", strings.TrimRight(s.BaseURL, "/"), detail.ID)
	var presign struct {
		URL string `json:"url"`
	}
	if err := postJSON(client, presignURL, map[string]any{"ttl_seconds": 120}, &presign); err != nil {
		return PulledArtifact{}, err
	}
	if presign.URL == "" {
		return PulledArtifact{}, fmt.Errorf("artifact %d presign url is empty", detail.ID)
	}

	var downloaded struct {
		ArtifactID int64           `json:"artifact_id"`
		Version    string          `json:"version"`
		Entries    []ArtifactEntry `json:"entries"`
	}
	if err := fetchJSON(client, presign.URL, &downloaded); err != nil {
		return PulledArtifact{}, err
	}
	if downloaded.ArtifactID == 0 {
		downloaded.ArtifactID = detail.ID
	}
	if downloaded.Version == "" {
		downloaded.Version = detail.Version
	}
	if len(downloaded.Entries) == 0 {
		return PulledArtifact{}, fmt.Errorf("artifact %d has no downloadable entries", detail.ID)
	}

	return PulledArtifact{
		ArtifactID: downloaded.ArtifactID,
		Version:    downloaded.Version,
		Entries:    downloaded.Entries,
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

func postJSON(client *http.Client, targetURL string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d for %s: %s", resp.StatusCode, targetURL, string(payload))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
