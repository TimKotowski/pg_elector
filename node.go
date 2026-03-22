package pg_elector

import (
	"os"
	"strings"
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
		host = "default_host"
	}

	if len(host) > maxHostLength {
		host = host[0:maxHostLength]
	}

	nodeId := strings.NewReplacer(".", "_", "-", "_").Replace(host)

	return nodeId + "_" + strings.ReplaceAll(time.Now().UTC().Format("2006_01_02T15_04_05Z07.00000"), ".", "_"), nil
}
