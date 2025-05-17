package deployment

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"strings"
)

type FunctionDeployer interface {
	Deploy(cfg *config.Configuration)
	Clean()
}

func CreateDeployer(cfg *config.Configuration) FunctionDeployer {
	switch strings.ToLower(cfg.LoaderConfiguration.Platform) {
	case common.PlatformAWSLambda:
		return newAWSLambdaDeployer()
	case common.PlatformDirigent:
		return newDirigentDeployer()
	case common.PlatformKnative:
		return newKnativeDeployer()
	case common.PlatformOpenWhisk:
		return newOpenWhiskDeployer()
	case common.PlatformGCR:
		return newGCRDeployer()
	default:
		logrus.Fatal("Unsupported platform.")
	}

	return nil
}
