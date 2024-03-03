package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"text/template"
)

type Lighthouse struct {
	name            string
	releasesUrl     string
	downloadUrl     string
	tagCommitShaUrl string
}

func NewLighthouseClient() *Lighthouse {
	return &Lighthouse{
		name:            "lighthouse",
		releasesUrl:     "https://api.github.com/repos/sigp/lighthouse/releases/latest",
		tagCommitShaUrl: "https://api.github.com/repos/sigp/lighthouse/git/refs/tags/{{.GIT_TAG}}",
		downloadUrl:     "https://github.com/sigp/lighthouse/releases/download/{{.GIT_TAG}}/lighthouse-{{.GIT_TAG}}-{{.ARCH}}-{{.OS}}.tar.gz",
	}
}

func (lighthouse *Lighthouse) Download() error {
	latestReleaseTag, err := lighthouse.fetchLatestReleaseTag()
	if err != nil {
		return err
	}

	log.Printf("Downloading Lighthouse %s for %s %s", latestReleaseTag, runtime.GOOS, runtime.GOARCH)

	os, err := lighthouse.getOs()
	if err != nil {
		return fmt.Errorf("error downloading Lighthouse: %w", err)
	}

	arch, err := lighthouse.getArch(os)
	if err != nil {
		return fmt.Errorf("error downloading Lighthouse: %w", err)
	}

	tmpl, err := template.New("downloadUrl").Parse(lighthouse.downloadUrl)
	if err != nil {
		return err
	}
	var downloadUrlBuf bytes.Buffer
	err = tmpl.Execute(&downloadUrlBuf, map[string]string{
		"OS":      os,
		"ARCH":    arch,
		"GIT_TAG": latestReleaseTag,
	})
	if err != nil {
		return err
	}

	downloadPath := fmt.Sprintf("./clients/binaries/%s/%s", lighthouse.name, latestReleaseTag)
	log.Println(downloadUrlBuf.String())
	err = downloadAndExtract(downloadUrlBuf.String(), downloadPath)
	if err != nil {
		return fmt.Errorf("there was an error downloading and extracting the Lighthouse binary: %w", err)
	}

	binaryPath := fmt.Sprintf("%s/%s", downloadPath, lighthouse.name)
	cmd := exec.Command(binaryPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("there was an error executing the --version command using the downloaded %s binary: %w", lighthouse.name, err)
	}

	log.Println("Successfully installed Lighthouse")

	return nil
}

func (lighthouse *Lighthouse) fetchLatestReleaseTag() (string, error) {
	response, err := http.Get(lighthouse.releasesUrl)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch %s's latest release tag: %s", lighthouse.name, response.Status)
	}

	var release githubReleaseApiResponse
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to decode %s's latest release tag response: %v", lighthouse.name, err)
	}

	return release.TagName, nil
}

func (lighthouse *Lighthouse) getOs() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "apple-darwin", nil
	case "linux":
		return "linux-gnu", nil
	default:
		return "", fmt.Errorf("unknown operating system: %s", runtime.GOOS)
	}
}

func (lighthouse *Lighthouse) getArch(os string) (string, error) {
	if os == "apple-darwin" {
		return "x86_64", nil
	}

	switch runtime.GOARCH {
	case "arm64":
		return "aarch64-unknown", nil
	case "x86_64":
		return "x86_64-unknown", nil
	default:
		return "", fmt.Errorf("unknown operating system: %s", runtime.GOARCH)
	}
}
