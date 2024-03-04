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
	"time"
)

type Geth struct {
	name            string
	releasesUrl     string
	downloadUrl     string
	tagCommitShaUrl string
	config          Config
}

type Config struct {
	rootPath string
	Network string  `json:"network"`
	DataDir string  `json:"datadir"`
	StdoutFile string `json:"stdoutFile"`
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
	Version string `json:"version"`
	OS string `json:"os"`
	Arch string `json:"arch"`
	ShaCommit string `json:"shaCommit"`
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

	downloadPath := fmt.Sprintf("%s/clients/binaries/%s/%s", geth.config.rootPath, geth.name, latestReleaseTag)
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

		geth.config.Binary.Path = fmt.Sprintf("clients/binaries/%s-%s-%s-%s-%s/geth", geth.name, runtime.GOOS, runtime.GOARCH, strings.Replace(latestReleaseTag, "v", "", 1), commitSha)
		geth.config.Binary.Version = latestReleaseTag
		geth.config.Binary.OS = runtime.GOOS
		geth.config.Binary.Arch = runtime.GOARCH
		geth.config.Binary.ShaCommit = commitSha
		// TODO Consider adding to Config struct
		writeConfig(geth.config, fmt.Sprintf("%s/clients/configs/geth.json", geth.config.rootPath))

		return nil
	}

	geth.getVersion(binaryPath)
	log.Printf("Geth already installed, using: %s", binaryPath)
	return nil
}

func (geth *Geth) Start() error {
	exists, err := checkIfPathExists(fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.AuthRPC.JWTSecret))
	if err != nil {
		return fmt.Errorf("error checking if JWT secret exists: %w", err)
	}
	if (!exists) {
		if err = geth.createJwtSecret(); err != nil {
			return err
		}
	}

	stdoutFile, err := os.OpenFile(fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.StdoutFile), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening file for Geth output: %w", err)
	}
	defer stdoutFile.Close()

	cmd := exec.Command(
		fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.Binary.Path),
		fmt.Sprintf("--%s", geth.config.Network),
		"--datadir",
		fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.DataDir),
		"--authrpc.addr",
		geth.config.AuthRPC.Addr,
		"--authrpc.port",
		geth.config.AuthRPC.Port,
		"--authrpc.vhosts",
		geth.config.AuthRPC.VHosts,
		"--authrpc.jwtsecret",
		fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.AuthRPC.JWTSecret),
		"--http",
		"--http.api",
		"eth,net",
	)
	cmd.Stdout = stdoutFile
	cmd.Stderr = stdoutFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting Geth: %w", err)
	}

	log.Printf("Successfully started Geth with process id: %d. Output redirected to %s", cmd.Process.Pid, fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.StdoutFile))

	geth.config.PID = cmd.Process.Pid
	if err = writeConfig(geth.config, fmt.Sprintf("%s/clients/configs/geth.json", geth.config.rootPath)); err != nil {
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
	if err = writeConfig(geth.config, fmt.Sprintf("%s/clients/configs/geth.json", geth.config.rootPath)); err != nil {
		return err
	}

	return nil
}

func (geth *Geth) Logs() error {
	logContent, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.StdoutFile))
	if err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}

	fmt.Print(string(logContent))

	if err = followLogs(fmt.Sprintf("%s/%s", geth.config.rootPath, geth.config.StdoutFile)); err != nil {
		return fmt.Errorf("error following log file: %w", err)
	}

	return nil
}

func parseConfig() (Config, error) {
	rootPath, err := findRootPath("./")
	if err != nil {
		return Config{}, err
	}

	configPath := fmt.Sprintf("%s/clients/configs/geth.json", rootPath)
	file, err := os.Open(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("failed to open Geth config file at %s: %w", configPath, err)
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read Geth config file at %s: %w", configPath, err)
	}

	var config Config
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse Geth config file at %s: %w", configPath, err)
	}

	config.rootPath = rootPath

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

	log.Printf("Successfully wrote config to: %s", path)
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

func followLogs(filePath string) error {
    // Open the log file.
    file, err := os.Open(filePath)
    if err != nil {
        return fmt.Errorf("error opening log file: %w", err)
    }
    defer file.Close()

    // Seek to the end of the file to start reading from the latest entry.
    _, err = file.Seek(0, io.SeekEnd)
    if err != nil {
        return fmt.Errorf("error seeking log file: %w", err)
    }

    // Continuously read from the file.
    for {
        // Attempt to read new content.
        data := make([]byte, 4096)
        n, err := file.Read(data)
        if err != nil && err != io.EOF {
            return fmt.Errorf("error reading log file: %w", err)
        }
        if n > 0 {
            fmt.Print(string(data[:n]))
        }

        // If we reach the end of the file (no new content), wait before trying again.
        if err == io.EOF {
            time.Sleep(time.Second)
        }
    }
}
