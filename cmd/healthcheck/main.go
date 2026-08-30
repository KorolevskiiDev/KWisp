// Command healthcheck probes the kwisp server's /healthz endpoint and exits
// with status 0 if the server responds OK. Used as the Docker HEALTHCHECK.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:8090/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck failed: unexpected status %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
