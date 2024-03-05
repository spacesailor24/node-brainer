package clients

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type githubReleaseApiResponse struct {
	TagName string `json:"tag_name"`
}

type githubTagCommitShaApiResponse struct {
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

// ProgressReader wraps an io.Reader to report progress.
type ProgressReader struct {
	r io.Reader // underlying reader
	totalRead int64 // total bytes read
	totalSize int64 // total size of the file
}

func checkIfPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// findRootPath will recursively search upwards for a go.mod file.
// This depends on the structure of the repo having a go.mod file at the root.
func findRootPath(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		modulePath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(modulePath); err == nil {
			return dir, nil
		}
		parentDir := filepath.Dir(dir)
		// Check if we reached the filesystem root
		if parentDir == dir {
			break
		}
		dir = parentDir
	}
	return "", fmt.Errorf("root not found")
}

// Read implements the io.Reader interface for ProgressReader.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.totalRead += int64(n)

	// Log progress
	percentComplete := float64(pr.totalRead) / float64(pr.totalSize) * 100
	fmt.Printf("\rDownloading... %.2f%% complete", percentComplete)

	return n, err
}

func downloadAndExtract(url, dest string) error {
	log.Printf("Downloading from: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check if we can determine the total size
	totalSize := resp.ContentLength
	if totalSize <= 0 {
		fmt.Println("Cannot determine the total size of the download.")
	}

	// Wrap the response body in a ProgressReader
	progressReader := &ProgressReader{
		r: resp.Body,
		totalSize: totalSize,
	}

	// Create a gzip reader
	gzr, err := gzip.NewReader(progressReader)
	if err != nil {
		return err
	}
	defer gzr.Close()

	// Create a tar reader
	tr := tar.NewReader(gzr)

	// Extract the tarball
	for {
		header, err := tr.Next()

		if err == io.EOF {
			fmt.Println("\nDownload and extraction complete.")
			break
		}
		if err != nil {
			return err
		}

		// Target extraction path
		target := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			parentDir := filepath.Dir(target)
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return err
			}

			outFile, err := os.Create(target)
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()

			if err := os.Chmod(target, 0755); err != nil {
				return err
			}
		}
	}
	return nil
}
