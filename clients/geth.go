package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	Binary  Binary  `json:"binary"`
	PID     int	`json:"pid"`
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

type Binary struct {
	Path string `json:"path"`
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
		return nil
	}

	geth.getVersion(binaryPath)
	log.Printf("Geth already installed, using: %s", binaryPath)
	return nil
}

func (geth *Geth) Start() error {
	exists, err := checkIfPathExists(geth.config.AuthRPC.JWTSecret)
	if err != nil {
		return fmt.Errorf("error checking if JWT secret exists: %w", err)
	}
	if (!exists) {
		if err = geth.createJwtSecret(); err != nil {
			return err
		}
	}

	tmpl, err := template.New("gethOptions").Parse("--{{.Network}} --datadir {{.DataDir}} --authrpc.addr {{.AuthRPC.Addr}} --authrpc.port {{.AuthRPC.Port}} --authrpc.vhosts {{.AuthRPC.VHosts}} --authrpc.jwtsecret {{.AuthRPC.JWTSecret}} --http --http.api eth,net")
	if err != nil {
		return fmt.Errorf("error creating Geth start command template: %w", err)
	}

	var startCmdBytes bytes.Buffer
	if err := tmpl.Execute(&startCmdBytes, geth.config); err != nil {
		return fmt.Errorf("error executing Geth start command template: %w", err)
	}

	cmd := exec.Command(
		geth.config.Binary.Path,
		fmt.Sprintf("--%s", geth.config.Network),
		"--datadir",
		geth.config.DataDir,
		"--authrpc.addr",
		geth.config.AuthRPC.Addr,
		"--authrpc.port",
		geth.config.AuthRPC.Port,
		"--authrpc.vhosts",
		geth.config.AuthRPC.VHosts,
		"--authrpc.jwtsecret",
		geth.config.AuthRPC.JWTSecret,
		"--http",
		"--http.api",
		"eth,net",
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting Geth: %w", err)
	}

	log.Printf("Successfully started Geth with process id: %d", cmd.Process.Pid)

	geth.config.PID = cmd.Process.Pid
	// TODO Un-hardcode
	if err = writeConfig(geth.config, "clients/configs/geth.json"); err != nil {
		return err
	}

	return nil
}

func (geth *Geth) Stop() error {
	if (geth.config.PID == -1) {
		// TODO Replace with a better check, perhaps search for running Geth instance?
		return fmt.Errorf("no Geth process running")
	}

	process, err := os.FindProcess(geth.config.PID)
	if err != nil {
		return fmt.Errorf("error finding Geth process with pid %d: %w", geth.config.PID, err)
	}

	// TODO Interrupt won't work on Windows
	if err = process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("error interrupting Geth process with pid %d: %w", geth.config.PID, err)
	}

	log.Printf("Successfully stopped Geth process with pid %d", geth.config.PID)
	
	geth.config.PID = -1
	// TODO Un-hardcode
	if err = writeConfig(geth.config, "clients/configs/geth.json"); err != nil {
		return err
	}

	return nil
}

func parseConfig() (Config, error) {
	// TODO Un-hardcode
	file, err := os.Open("clients/configs/geth.json")
	if err != nil {
		return Config{}, fmt.Errorf("failed to open Geth config file at ./configs/geth.json: %w", err)
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
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

func writeConfig(config Config, path string) error {
	data, err := json.MarshalIndent(config, "", " ")
	if err != nil {
		return fmt.Errorf("error marshaling provided config to JSON: %w", err)
	}

	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("error writing config to %s: %w", path, err)
	}

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

func (geth *Geth) createJwtSecret() error {
	cmd := exec.Command("openssl", "rand", "-hex", "32")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error generating JWT secret: %w", err)
	}

	if err := ioutil.WriteFile(geth.config.AuthRPC.JWTSecret, output, 0644); err != nil {
		return fmt.Errorf("error writing JWT secret to %s: %w", geth.config.AuthRPC.JWTSecret, err)
	}

	exists, err := checkIfPathExists(geth.config.AuthRPC.JWTSecret)
	if err != nil {
		return fmt.Errorf("error checking if JWT secret exists after creating it: %w", err)
	}
	if (!exists) {
		return fmt.Errorf("JWT secret still doesn't exist at %s, even after attempting to create it", geth.config.AuthRPC.JWTSecret)
	}

	return nil
}
