package common

import (
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
)

func TestSweepTypeValidator(t *testing.T) {
	t.Run("Check Grid Sweep Type", func(t *testing.T) {
		CheckSweepType("grid")
	})
	t.Run("Check Random Sweep Type", func(t *testing.T) {
		expectFatal(t, func() {
			CheckSweepType("random")
		})
	})
	t.Run(("Check Linear Sweep Type"), func(t *testing.T) {
		CheckSweepType("linear")
	})

}

func expectFatal(t *testing.T, funcToTest func()) {
	fatal := false
	originalExitFunc := log.StandardLogger().ExitFunc
	log.Info("Expecting a fatal message during the test, overriding the exit function")
	// Replace logrus exit function
	log.StandardLogger().ExitFunc = func(int) {
		fatal = true
		t.SkipNow()
	}
	defer func() {
		log.StandardLogger().ExitFunc = originalExitFunc
		assert.True(t, fatal, "Expected log.Fatal to be called")
	}()
	funcToTest()
}
