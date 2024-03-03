package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
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
	config          Config
}

type Config struct {
	Network string  `json:"network"`
	DataDir string  `json:"datadir"`
	AuthRPC AuthRPC `json:"authrpc"`
	HTTP    HTTP    `json:"http"`
}

type AuthRPC struct {
	Addr      string `json:"addr"`
	Port      string `json:"port"`
	VHosts    string `json:"vhosts"`
	JWTSecret string `json:"jwtsecret"`
}

type HTTP struct {
	Enabled bool     `json:"enabled"`
	API     []string `json:"api"`
}

func NewGethClient() (*Geth, error) {
	config, err := parseConfig()
	if err != nil {
		return &Geth{}, fmt.Errorf("failed to create new Geth client: %w", err)
	}

	return &Geth{
		name:            "geth",
		releasesUrl:     "https://api.github.com/repos/ethereum/go-ethereum/releases/latest",
		tagCommitShaUrl: "https://api.github.com/repos/ethereum/go-ethereum/git/refs/tags/{{.GIT_TAG}}",
		downloadUrl:     "https://gethstore.blob.core.windows.net/builds/geth-{{.OS}}-{{.ARCH}}-{{.GIT_TAG}}-{{.GIT_COMMIT}}.tar.gz",
		config:          config,
	}, nil
}

func parseConfig() (Config, error) {
	file, err := os.Open("clients/configs/geth.json")
	if err != nil {
		return Config{}, fmt.Errorf("failed to open Geth config file at ./configs/geth.json: %w", err)
	}
	defer file.Close()

	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read Geth config file at ./configs/geth.json: %w", err)
	}

	var config Config
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse Geth config file at ./configs/geth.json: %w", err)
	}

	return config, nil
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

	downloadPath := fmt.Sprintf("./clients/binaries/%s/%s", geth.name, latestReleaseTag)

	binaryPath := fmt.Sprintf("%s/%s-%s-%s-%s-%s/geth", downloadPath, geth.name, runtime.GOOS, runtime.GOARCH, strings.Replace(latestReleaseTag, "v", "", 1), commitSha)
	exists, err := checkIfPathExists(binaryPath)
	if err != nil {
		return fmt.Errorf("there was an error checking if Geth binary exists at %s: %w", binaryPath, err)
	}

	if !exists {
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

		err = downloadAndExtract(downloadUrlBuf.String(), downloadPath)
		if err != nil {
			return fmt.Errorf("there was an error downloading and extracting the Geth binary: %w", err)
		}

		geth.getVersion(binaryPath)
		log.Println("Successfully installed Geth")
	}

	geth.getVersion(binaryPath)
	log.Printf("Geth already installed, using: %s", binaryPath)

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

func (geth *Geth) getVersion(binaryPath string) error {
	cmd := exec.Command(binaryPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("there was an error executing the --version command using the downloaded %s binary: %w", geth.name, err)
	}

	return nil
}
