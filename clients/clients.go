package clients

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type clientMetadata struct {
	name string
	releasesUrl string
	downloadUrl string
}

type githubReleaseApiResponse struct {
    TagName string `json:"tag_name"`
}

// https://github.com/sigp/lighthouse/releases/download/v4.5.444-exp/lighthouse-v4.5.444-exp-x86_64-apple-darwin.tar.gz

func (clientMetadata *clientMetadata) fetchLatestReleaseTag() (string, error) {
	response, err := http.Get(clientMetadata.releasesUrl)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Failed to fetch %s's latest release tag: %s", clientMetadata.name, response.Status)
	}

	var release githubReleaseApiResponse
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("Failed to decode %s's latest release tag response: %v", clientMetadata.name, err)
	}

	return release.TagName, nil
}
