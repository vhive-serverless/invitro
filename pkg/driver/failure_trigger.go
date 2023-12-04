package driver

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/config"
	"os/exec"
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

func triggerKnativeFailure(node string, component string, t int) bool {
	panic("Not yet implemented")
}

func triggerDirigentFailure(node string, component string, t int) bool {
	time.Sleep(time.Duration(t) * time.Second)

	logrus.Infof("Failure triggered...")

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

	if node != "" {
		command = append([]string{"ssh", "-i", "~/.ssh/cl", node}, command...)
	}

	cmd := exec.Command(command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logrus.Errorf("Error triggering %s failure at t = %d - %v", command, t, err)
		return false
	}

	logrus.Infof("Failure triggered - %s", string(output))
	return true
}
