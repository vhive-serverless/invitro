package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Cell struct {
	ID, Experiment, Profile, Mode, Workload, Status string
	Repetition                                      int
	ConfigHash                                      string `json:"config_sha256"`
}

func CellID(experiment, profile, mode, workload string, repetition int) string {
	return fmt.Sprintf("%s/%s/%s/%s/r%d", experiment, profile, mode, workload, repetition)
}
func NewCell(experiment, profile, mode, workload string, repetition int) Cell {
	return Cell{ID: CellID(experiment, profile, mode, workload, repetition), Experiment: experiment, Profile: profile, Mode: mode, Workload: workload, Repetition: repetition, Status: "planned"}
}
func SHA256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func CreateOnly(path string, value any) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
