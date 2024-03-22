package driver

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/config"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func scheduleFailure(config *config.LoaderConfiguration) {
	if config.FailAt != 0 && config.FailComponent != "" {
		switch config.Platform {
		case "Knative", "Knative-RPS":
			triggerKnativeFailure(config.FailNode, config.FailComponent, config.FailAt)
		case "Dirigent", "Dirigent-RPS":
			triggerDirigentFailure(config.FailNode, config.FailComponent, config.FailAt)
		}
	}
}

func triggerKnativeFailure(_ string, component string, t int) bool {
	time.Sleep(time.Duration(t) * time.Second)

	var command []string
	switch component {
	case "control_plane":
		command = []string{"bash", "./pkg/driver/knative_delete_control_plane.sh"}
	case "data_plane":
		command = []string{"bash", "./pkg/driver/knative_delete_data_plane.sh"}
	case "worker_node":
		panic("Not yet implemented")
	default:
		logrus.Fatal("Invalid component to fail.")
	}

	invokeCommand(command, t)
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
