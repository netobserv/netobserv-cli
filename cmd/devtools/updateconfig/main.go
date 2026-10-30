// Command updateconfig refreshes internal/collector/config.yaml from the network-observability-operator repo,
// replacing scripts/update-config.sh. Invoked via `make update-config`; not shipped to users.
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	sourceURL = "https://raw.githubusercontent.com/netobserv/network-observability-operator/refs/heads/main/controllers/consoleplugin/config/static-frontend-config.yaml"
	destPath  = "./internal/collector/config.yaml"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	fmt.Println("Downloading frontend config from operator repo")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return fmt.Errorf("cannot fetch %s: %w", sourceURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status fetching %s: %s", sourceURL, resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", destPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("cannot write %s: %w", destPath, err)
	}
	return nil
}
