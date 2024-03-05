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
	"text/template"
)

type Lighthouse struct {
	name            string
	releasesUrl     string
	downloadUrl     string
	tagCommitShaUrl string
	config          LighthouseConfig
}

type LighthouseConfig struct {
	rootPath          string
	Network           string    `json:"network"`
	DataDir           string    `json:"datadir"`
	StdoutFile        string    `json:"stdoutFile"`
	Execution         Execution `json:"execution"`
	CheckpointSyncUrl string    `json:"checkpointSyncUrl"`
	Http              bool      `json:"http"`
	Binary            Binary    `json:"binary"`
	PID               int       `json:"pid"`
}

type Execution struct {
	Endpoint string `json:"endpoint"`
	JWT      string `json:"jwt"`
}

func NewLighthouseClient() (*Lighthouse, error) {
	config, err := parseLighthouseConfig()
	if err != nil {
		return &Lighthouse{}, fmt.Errorf("failed to create new Geth client: %w", err)
	}

	return &Lighthouse{
		name:            "lighthouse",
		releasesUrl:     "https://api.github.com/repos/sigp/lighthouse/releases/latest",
		tagCommitShaUrl: "https://api.github.com/repos/sigp/lighthouse/git/refs/tags/{{.GIT_TAG}}",
		downloadUrl:     "https://github.com/sigp/lighthouse/releases/download/{{.GIT_TAG}}/lighthouse-{{.GIT_TAG}}-{{.ARCH}}-{{.OS}}.tar.gz",
		config:          config,
	}, nil
}

func (lighthouse *Lighthouse) Download() error {
	latestReleaseTag, err := lighthouse.fetchLatestReleaseTag()
	if err != nil {
		return err
	}

	downloadPath := fmt.Sprintf("./clients/binaries/%s/%s", lighthouse.name, latestReleaseTag)
	binaryPath := fmt.Sprintf("%s/%s", downloadPath, lighthouse.name)
	exists, err := checkIfPathExists(binaryPath)
	if err != nil {
		return fmt.Errorf("there was an error checking if Lighthouse binary exists at %s: %w", binaryPath, err)
	}

	if !exists {
		log.Printf("Downloading Lighthouse %s for %s %s", latestReleaseTag, runtime.GOOS, runtime.GOARCH)

		os, err := lighthouse.getOs()
		if err != nil {
			return fmt.Errorf("error downloading Lighthouse: %w", err)
		}

		arch, err := lighthouse.getArch(os)
		if err != nil {
			return fmt.Errorf("error downloading Lighthouse: %w", err)
		}

		if lighthouse.shouldUsePortable(os) {
			os = fmt.Sprintf("%s-portable", os)
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

		err = downloadAndExtract(downloadUrlBuf.String(), downloadPath)
		if err != nil {
			return fmt.Errorf("there was an error downloading and extracting the Lighthouse binary: %w", err)
		}

		if err = lighthouse.getVersion(binaryPath); err != nil {
			return err
		}
		log.Println("Successfully installed Lighthouse")

		lighthouse.config.Binary.Path = fmt.Sprintf("clients/binaries/%s/%s/lighthouse", lighthouse.name, latestReleaseTag)
		lighthouse.config.Binary.Version = latestReleaseTag
		lighthouse.config.Binary.OS = os
		lighthouse.config.Binary.Arch = arch
		writeLighthouseConfig(lighthouse.config, fmt.Sprintf("%s/clients/configs/lighthouse.json", lighthouse.config.rootPath))

		return nil
	}

	if err = lighthouse.getVersion(binaryPath); err != nil {
		return err
	}
	log.Printf("Lighthouse already installed, using: %s", binaryPath)

	return nil
}

func (lighthouse *Lighthouse) Start() error {
	exists, err := checkIfPathExists(fmt.Sprintf("%s/%s", lighthouse.config.rootPath, lighthouse.config.Execution.JWT))
	if err != nil {
		return fmt.Errorf("error checking if JWT secret exists: %w", err)
	}
	if !exists {
		if err = lighthouse.createJwtSecret(); err != nil {
			return err
		}
	}

	stdoutFile, err := os.OpenFile(fmt.Sprintf("%s/%s", lighthouse.config.rootPath, lighthouse.config.StdoutFile), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening file for Lighthouse output: %w", err)
	}
	defer stdoutFile.Close()

	cmd := exec.Command(
		fmt.Sprintf("%s/%s", lighthouse.config.rootPath, lighthouse.config.Binary.Path),
		"bn",
		"--datadir", lighthouse.config.DataDir,
		"--network", lighthouse.config.Network,
		"--execution-endpoint", lighthouse.config.Execution.Endpoint,
		"--execution-jwt", lighthouse.config.Execution.JWT,
		"--checkpoint-sync-url", lighthouse.config.CheckpointSyncUrl,
		"--http",
		"--disable-deposit-contract-sync",
	)
	log.Println(cmd.String())
	cmd.Stdout = stdoutFile
	cmd.Stderr = stdoutFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting Lighthouse: %w", err)
	}

	log.Printf("Successfully started Lighthouse with process id: %d. Output redirected to %s", cmd.Process.Pid, fmt.Sprintf("%s/%s", lighthouse.config.rootPath, lighthouse.config.StdoutFile))

	lighthouse.config.PID = cmd.Process.Pid
	if err = writeLighthouseConfig(lighthouse.config, fmt.Sprintf("%s/clients/configs/lighthouse.json", lighthouse.config.rootPath)); err != nil {
		return err
	}

	return nil
}

func (lighthouse *Lighthouse) Stop() error {
	if lighthouse.config.PID == -1 {
		// TODO Replace with a better check, perhaps search for running Geth instance?
		return fmt.Errorf("no Lighthouse process running")
	}

	process, err := os.FindProcess(lighthouse.config.PID)
	if err != nil {
		return fmt.Errorf("error finding Geth process with pid %d: %w", lighthouse.config.PID, err)
	}

	// TODO Interrupt won't work on Windows
	if err = process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("error interrupting Lighthouse process with pid %d: %w", lighthouse.config.PID, err)
	}

	log.Printf("Successfully stopped Lighthouse process with pid %d", lighthouse.config.PID)

	lighthouse.config.PID = -1
	if err = writeLighthouseConfig(lighthouse.config, fmt.Sprintf("%s/clients/configs/lighthouse.json", lighthouse.config.rootPath)); err != nil {
		return err
	}

	return nil
}

func (lighthouse *Lighthouse) Logs() error {
	logContent, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", lighthouse.config.rootPath, lighthouse.config.StdoutFile))
	if err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}

	fmt.Print(string(logContent))

	if err = followLogs(fmt.Sprintf("%s/%s", lighthouse.config.rootPath, lighthouse.config.StdoutFile)); err != nil {
		return fmt.Errorf("error following log file: %w", err)
	}

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

func (lighthouse *Lighthouse) shouldUsePortable(os string) bool {
	if os == "apple-darwin" && runtime.GOARCH == "arm64" {
		return true
	}

	return false
}

func (lighthouse *Lighthouse) getVersion(binaryPath string) error {
	cmd := exec.Command(binaryPath, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("there was an error executing the --version command using the downloaded %s binary: %w", lighthouse.name, err)
	}

	return nil
}

func (lighthouse *Lighthouse) createJwtSecret() error {
	cmd := exec.Command("openssl", "rand", "-hex", "32")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error generating JWT secret: %w", err)
	}

	if err := ioutil.WriteFile(lighthouse.config.Execution.JWT, output, 0644); err != nil {
		return fmt.Errorf("error writing JWT secret to %s: %w", lighthouse.config.Execution.JWT, err)
	}

	exists, err := checkIfPathExists(lighthouse.config.Execution.JWT)
	if err != nil {
		return fmt.Errorf("error checking if JWT secret exists after creating it: %w", err)
	}
	if !exists {
		return fmt.Errorf("JWT secret still doesn't exist at %s, even after attempting to create it", lighthouse.config.Execution.JWT)
	}

	return nil
}

func parseLighthouseConfig() (LighthouseConfig, error) {
	rootPath, err := findRootPath("./")
	if err != nil {
		return LighthouseConfig{}, err
	}

	configPath := fmt.Sprintf("%s/clients/configs/lighthouse.json", rootPath)
	file, err := os.Open(configPath)
	if err != nil {
		return LighthouseConfig{}, fmt.Errorf("failed to open Lighthouse config file at %s: %w", configPath, err)
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	if err != nil {
		return LighthouseConfig{}, fmt.Errorf("failed to read Lighthouse config file at %s: %w", configPath, err)
	}

	var config LighthouseConfig
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		return LighthouseConfig{}, fmt.Errorf("failed to parse Lighthouse config file at %s: %w", configPath, err)
	}

	config.rootPath = rootPath

	return config, nil
}

func writeLighthouseConfig(config LighthouseConfig, path string) error {
	data, err := json.MarshalIndent(config, "", " ")
	if err != nil {
		return fmt.Errorf("error marshaling provided config to JSON: %w", err)
	}

	err = ioutil.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("error writing config to %s: %w", path, err)
	}

	log.Printf("Successfully wrote config to: %s", path)
	return nil
}
