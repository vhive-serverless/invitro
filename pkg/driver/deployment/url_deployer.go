package deployment

import (
	"os"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/loader/pkg/config"
)

type urlDeployer struct{}

func newURLDeployer() *urlDeployer {
	return &urlDeployer{}
}

func (*urlDeployer) Deploy(cfg *config.Configuration) {
	filePath := cfg.LoaderConfiguration.TracePath + "/endpoints.txt"
	if cfg.LoaderConfiguration.TracePath == "RPS" {
		filePath = "endpoints.txt"
	}

	file, err := os.ReadFile(filePath)

	if err != nil {
		log.Fatalf("URL file not found: %s, err=%e", filePath, err)
	}

	endpoints := strings.Split(string(file), "\n")
	if endpoints[len(endpoints)-1] == "" { // remove last empty line
		endpoints = endpoints[:len(endpoints)-1]
	}
	log.Debugf("Using endpoints: %v", endpoints)

	if len(endpoints) != len(cfg.Functions) {
		log.Fatalf("Number of endpoints does not match number of functions: %d != %d", len(endpoints), len(cfg.Functions))
	}

	for i, endpoint := range endpoints {
		cfg.Functions[i].Endpoint = endpoint
	}
}

func (*urlDeployer) Clean() {
}
