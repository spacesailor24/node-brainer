package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
)

type GethClient struct {
	*clientMetadata
	tagCommitShaUrl string
}

type githubTagCommitShaApiResponse struct {
    Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

func NewGethClient() *GethClient {
	return &GethClient{
		clientMetadata: &clientMetadata{
			name:        "geth",
			releasesUrl: "https://api.github.com/repos/ethereum/go-ethereum/releases/latest",
			downloadUrl: "https://gethstore.blob.core.windows.net/builds/geth-{{.OS}}-{{.ARCH}}-{{.GIT_TAG}}-{{.GIT_COMMIT}}.tar.gz",
		},
		tagCommitShaUrl: "https://api.github.com/repos/ethereum/go-ethereum/git/refs/tags/{{.GIT_TAG}}",
	}
}

func (gethClient *GethClient) fetchTagCommitSha(latestReleaseTag string) (string, error) {
	tmpl, err := template.New("tagCommitShaUrl").Parse(gethClient.tagCommitShaUrl)
	if err != nil {
		return "", err
	}

	var tagCommitShaUrlBuf bytes.Buffer
	err = tmpl.Execute(&tagCommitShaUrlBuf, map[string]string{
		"GIT_TAG": latestReleaseTag,
	})
	if err != nil {
		return "", err
	}

	response, err := http.Get(tagCommitShaUrlBuf.String())
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Failed to fetch %s's commit hash for %s tag: %s", gethClient.name, latestReleaseTag, response.Status)
	}

	var commitSha githubTagCommitShaApiResponse
	if err := json.NewDecoder(response.Body).Decode(&commitSha); err != nil {
		return "", fmt.Errorf("Failed to decode %s's latest release tag response: %v", gethClient.name, err)
	}

	return commitSha.Object.SHA[:8], nil
}
