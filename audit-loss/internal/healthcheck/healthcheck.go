package healthcheck

import (
	"net/http"
	"os"
	"time"
)

func MaybeExit() {
	if len(os.Args) < 3 || os.Args[1] != "-healthcheck" {
		return
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(os.Args[2])
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}
