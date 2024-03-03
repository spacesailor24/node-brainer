package clients

import (
	"archive/tar"
	"compress/gzip"
	"io"
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

func downloadAndExtract(url, dest string) error {
	// Download the file
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create a gzip reader
	gzr, err := gzip.NewReader(resp.Body)
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
			break
		}
		if err != nil {
			return err
		}

		// Target extraction path
		target := filepath.Join(dest, header.Name)

		// TODO Was causing some issues running binary after extracting
		// Modify header.Name to remove the base directory portion.
		// This splits the path and uses all but the first element (the base directory).
		// parts := strings.SplitN(header.Name, "/", 2)
		// if len(parts) < 2 {
		//     // Skip the base directory itself, or continue based on specific needs.
		//     continue
		// }
		// modifiedName := parts[1]
		// target := filepath.Join(dest, modifiedName)

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
