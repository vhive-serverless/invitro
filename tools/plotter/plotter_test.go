package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPlotter(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	records := parseFiles("./test-data")

	log.Debugf("Obtained %d records.", len(records))
	require.Equal(t, len(records), 2)

	plotFig("./test-out", records)
}