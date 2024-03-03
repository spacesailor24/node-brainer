package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"text/template"
)

type Geth struct {
	name            string
	releasesUrl     string
	downloadUrl     string
	tagCommitShaUrl string
}

type githubReleaseApiResponse struct {
	TagName string `json:"tag_name"`
}

type githubTagCommitShaApiResponse struct {
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

func NewGethClient() *Geth {
	return &Geth{
		name:            "geth",
		releasesUrl:     "https://api.github.com/repos/ethereum/go-ethereum/releases/latest",
		tagCommitShaUrl: "https://api.github.com/repos/ethereum/go-ethereum/git/refs/tags/{{.GIT_TAG}}",
		downloadUrl:     "https://gethstore.blob.core.windows.net/builds/geth-{{.OS}}-{{.ARCH}}-{{.GIT_TAG}}-{{.GIT_COMMIT}}.tar.gz",
	}
}

func (geth *Geth) Download() error {
	latestReleaseTag, err := geth.fetchLatestReleaseTag()
	if err != nil {
		return err
	}

	commitSha, err := geth.fetchTagCommitSha(latestReleaseTag)
	if err != nil {
		return err
	}

	log.Printf("Downloading Geth %s %s for %s %s", latestReleaseTag, commitSha, runtime.GOOS, runtime.GOARCH)

	tmpl, err := template.New("downloadUrl").Parse(geth.downloadUrl)
	if err != nil {
		return err
	}
	var downloadUrlBuf bytes.Buffer
	err = tmpl.Execute(&downloadUrlBuf, map[string]string{
		"OS":         runtime.GOOS,
		"ARCH":       runtime.GOARCH,
		"GIT_TAG":    strings.Replace(latestReleaseTag, "v", "", 1),
		"GIT_COMMIT": commitSha,
	})
	if err != nil {
		return err
	}

	downloadPath := fmt.Sprintf("./clients/binaries/%s/%s", geth.name, latestReleaseTag)
	err = downloadAndExtract(downloadUrlBuf.String(), downloadPath)
	if err != nil {
		return fmt.Errorf("there was an error downloading and extracting the Geth binary: %w", err)
	}

	binaryPath := fmt.Sprintf("%s/%s-%s-%s-%s-%s/geth", downloadPath, geth.name, runtime.GOOS, runtime.GOARCH, strings.Replace(latestReleaseTag, "v", "", 1), commitSha)
	log.Println(binaryPath)
	cmd := exec.Command(binaryPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("there was an error executing the --version command using the downloaded %s binary: %w", geth.name, err)
	}

	log.Println("Successfully installed Geth")

	return nil
}

func (geth *Geth) fetchLatestReleaseTag() (string, error) {
	response, err := http.Get(geth.releasesUrl)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch %s's latest release tag: %s", geth.name, response.Status)
	}

	var release githubReleaseApiResponse
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to decode %s's latest release tag response: %v", geth.name, err)
	}

	return release.TagName, nil
}

func (geth *Geth) fetchTagCommitSha(latestReleaseTag string) (string, error) {
	tmpl, err := template.New("tagCommitShaUrl").Parse(geth.tagCommitShaUrl)
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
		return "", fmt.Errorf("failed to fetch %s's commit hash for %s tag: %s", geth.name, latestReleaseTag, response.Status)
	}

	var commitSha githubTagCommitShaApiResponse
	if err := json.NewDecoder(response.Body).Decode(&commitSha); err != nil {
		return "", fmt.Errorf("failed to decode %s's latest release tag response: %v", geth.name, err)
	}

	return commitSha.Object.SHA[:8], nil
}
