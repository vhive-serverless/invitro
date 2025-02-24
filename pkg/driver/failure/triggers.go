package failure

import (
	"github.com/vhive-serverless/loader/pkg/common"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/config"
)

const (
	NodeSeparator = " "

	ControlPlaneFailure = "control_plane"
	DataPlaneFailure    = "data_plane"
	WorkerNodeFailure   = "worker_node"
)

func ScheduleFailure(platform string, config *config.FailureConfiguration) {
	if config != nil && config.FailureEnabled && config.FailAt != 0 && config.FailComponent != "" {
		time.Sleep(time.Duration(config.FailAt) * time.Second)

		switch platform {
		case common.PlatformKnative:
			triggerKnativeFailure(config.FailNode, config.FailComponent)
		case common.PlatformDirigent:
			triggerDirigentFailure(config.FailNode, config.FailComponent)
		default:
			logrus.Errorf("No specified failure handler for given type of system.")
		}
	}
}

func invokeRemotely(command []string, nodes string) {
	splitNodes := strings.Split(nodes, NodeSeparator)
	wg := &sync.WaitGroup{}

	for _, node := range splitNodes {
		wg.Add(1)

		go func(command []string, node string) {
			defer wg.Done()

			finalCommand := append([]string{"ssh", "-o", "StrictHostKeyChecking=no", node}, command...)
			invokeLocally(finalCommand)
		}(command, node)
	}

	wg.Wait()
}

func triggerKnativeFailure(nodes string, component string) {
	var command []string
	switch component {
	case ControlPlaneFailure:
		command = []string{"bash", "./pkg/driver/failure/knative_delete_control_plane.sh"}
	case DataPlaneFailure:
		command = []string{"bash", "./pkg/driver/failure/knative_delete_data_plane.sh"}
	case WorkerNodeFailure:
		command = []string{"sudo", "systemctl", "restart", "kubelet"}
	default:
		logrus.Fatal("Invalid component to fail.")
	}

	if component != "worker_node" {
		invokeLocally(command)
	} else {
		invokeRemotely(command, nodes)
	}
}

func triggerDirigentFailure(nodes string, component string) {
	var command []string
	switch component {
	case ControlPlaneFailure:
		command = []string{"sudo", "systemctl", "restart", "control_plane"}
	case DataPlaneFailure:
		command = []string{"sudo", "systemctl", "restart", "data_plane"}
	case WorkerNodeFailure:
		command = []string{"sudo", "systemctl", "restart", "worker_node"}
	default:
		logrus.Fatal("Invalid component to fail.")
	}

	if nodes == "" {
		invokeLocally(command)
	} else {
		invokeRemotely(command, nodes)
	}
}

func invokeLocally(command []string) {
	cmd := exec.Command(command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("Error triggering %s failure - %v", command, err)
		return
	}

	logrus.Infof("Failure triggered - %s", string(output))
}
