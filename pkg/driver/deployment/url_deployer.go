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
	file, err := os.ReadFile(cfg.LoaderConfiguration.TracePath + "/endpoints.txt")

	if err != nil {
		log.Fatalf("URL file not found: %s, err=%e", cfg.LoaderConfiguration.TracePath+"/endpoints.txt", err)
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
