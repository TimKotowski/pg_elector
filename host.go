package pg_elector

import (
	"os"
	"time"
)

func getNodeId() (string, error) {
	// Don't allow super long host names, narrow it down.
	maxHostLength := 80
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}

	if host == "" {
		host = "default"
	}

	if len(host) > maxHostLength {
		host = host[0:maxHostLength]
	}

	return host + "_" + time.Now().UTC().Format("2006_01_02T15_04_05Z07.00000"), nil
}
