package test

import (
	util "github.com/eth-easl/loader/pkg/common"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadIntArray(t *testing.T) {
	file := "../../data/coldstarts/test.csv"
	coldstarts := util.ReadIntArray(file, ",")
	assert.Equal(t,
		[]int{2, 0, 0, 0, 1, 0, 1, 1, 0, 0, 1, 3, 1, 1, 0, 0, 0, 0, 2, 0, 0, 0, 1, 0, 1, 1, 0, 0, 1, 3},
		coldstarts,
	)
}
