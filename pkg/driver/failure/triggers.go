package failure

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/config"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const NodeSeparator = " "

func ScheduleFailure(platform string, config *config.FailureConfiguration) {
	if config != nil && config.FailAt != 0 && config.FailComponent != "" {
		switch platform {
		case "Knative", "Knative-RPS":
			triggerKnativeFailure(config.FailNode, config.FailComponent, config.FailAt)
		case "Dirigent", "Dirigent-RPS":
			triggerDirigentFailure(config.FailNode, config.FailComponent, config.FailAt)
		default:
			logrus.Errorf("No specified failure handler for given type of system.")
		}
	}
}

func triggerKnativeFailure(nodes string, component string, t int) {
	time.Sleep(time.Duration(t) * time.Second)

	var command []string
	switch component {
	case "control_plane":
		command = []string{"bash", "./pkg/driver/failure/knative_delete_control_plane.sh"}
	case "data_plane":
		command = []string{"bash", "./pkg/driver/failure/knative_delete_data_plane.sh"}
	case "worker_node":
		command = []string{"sudo", "systemctl", "restart", "kubelet"}
	default:
		logrus.Fatal("Invalid component to fail.")
	}

	if component != "worker_node" {
		invokeCommand(command, t)
	} else {
		splitNodes := strings.Split(nodes, NodeSeparator)
		wg := &sync.WaitGroup{}

		for _, node := range splitNodes {
			wg.Add(1)

			go func(command []string, node string, t int) {
				defer wg.Done()

				finalCommand := append([]string{"ssh", "-o", "StrictHostKeyChecking=no", node}, command...)
				invokeCommand(finalCommand, t)
			}(command, node, t)
		}

		wg.Wait()
	}
}

func triggerDirigentFailure(nodes string, component string, t int) {
	time.Sleep(time.Duration(t) * time.Second)

	var command []string
	switch component {
	case "control_plane":
		command = []string{"sudo", "systemctl", "restart", "control_plane"}
	case "data_plane":
		command = []string{"sudo", "systemctl", "restart", "data_plane"}
	case "worker_node":
		command = []string{"sudo", "systemctl", "restart", "worker_node"}
	default:
		logrus.Fatal("Invalid component to fail.")
	}

	if nodes == "" {
		invokeCommand(command, t)
	} else {
		splitNodes := strings.Split(nodes, " ")
		wg := &sync.WaitGroup{}

		for _, node := range splitNodes {
			wg.Add(1)

			go func(command []string, node string, t int) {
				defer wg.Done()

				finalCommand := append([]string{"ssh", "-o", "StrictHostKeyChecking=no", node}, command...)
				invokeCommand(finalCommand, t)
			}(command, node, t)
		}

		wg.Wait()
	}
}

func invokeCommand(command []string, t int) {
	cmd := exec.Command(command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("Error triggering %s failure at t = %d - %v", command, t, err)
		return
	}

	logrus.Infof("Failure triggered - %s", string(output))
}
