package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	rpc "github.com/eth-easl/loader/server"
)

func TestExecute(t *testing.T) {
	server := funcServer{}
	req := rpc.FaasRequest{
		Message:           "",
		RuntimeInMilliSec: 0,
		MemoryInMebiBytes: 1,
	}

	_, err := server.Execute(context.TODO(), &req)
	assert.EqualError(t, err, "erroneous request")

	req.RuntimeInMilliSec = 1
	req.MemoryInMebiBytes = 600

	_, err = server.Execute(context.TODO(), &req)
	assert.EqualError(t, err, "erroneous request")
}
