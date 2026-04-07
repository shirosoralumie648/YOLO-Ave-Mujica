package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ResolvedArtifact struct {
	ArtifactID  int64
	Version     string
	DownloadURL string
}

type APIArtifactSource struct {
	BaseURL    string
	HTTPClient *http.Client
}

var ErrArtifactUnavailable = errors.New("artifact unavailable")

type httpStatusError struct {
	StatusCode int
	URL        string
	Body       string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("unexpected status %d for %s: %s", e.StatusCode, e.URL, e.Body)
}

func NewAPIArtifactSource(baseURL string) *APIArtifactSource {
	return &APIArtifactSource{BaseURL: baseURL}
}

func (s *APIArtifactSource) ResolveArtifact(dataset, format, version string) (ResolvedArtifact, error) {
	client := s.httpClient()
	resolveURL := fmt.Sprintf("%s/v1/artifacts/resolve?format=%s&version=%s",
		strings.TrimRight(s.BaseURL, "/"),
		url.QueryEscape(format),
		url.QueryEscape(version),
	)
	if dataset != "" {
		resolveURL += "&dataset=" + url.QueryEscape(dataset)
	}

	var artifact struct {
		ID          int64  `json:"id"`
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
	}
	if err := fetchJSON(client, resolveURL, &artifact); err != nil {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return ResolvedArtifact{}, fmt.Errorf("%w: %s", ErrArtifactUnavailable, err)
		}
		return ResolvedArtifact{}, err
	}

	downloadURL := artifact.DownloadURL
	if downloadURL == "" {
		downloadURL = fmt.Sprintf("/v1/artifacts/%d/download", artifact.ID)
	}
	return ResolvedArtifact{
		ArtifactID:  artifact.ID,
		Version:     artifact.Version,
		DownloadURL: s.absoluteURL(downloadURL),
	}, nil
}

func (s *APIArtifactSource) DownloadArchive(ctx context.Context, artifact ResolvedArtifact, tempPath string) error {
	return downloadArchiveToTemp(ctx, s.httpClient(), s.absoluteURL(artifact.DownloadURL), tempPath)
}

func (s *APIArtifactSource) httpClient() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return http.DefaultClient
}

func (s *APIArtifactSource) absoluteURL(target string) string {
	if target == "" {
		return target
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	base := strings.TrimRight(s.BaseURL, "/")
	if strings.HasPrefix(target, "/") {
		return base + target
	}
	return base + "/" + target
}

func fetchJSON(client *http.Client, targetURL string, out any) error {
	resp, err := client.Get(targetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &httpStatusError{StatusCode: resp.StatusCode, URL: targetURL, Body: string(body)}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
