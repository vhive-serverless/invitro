package test

import (
	"sort"
	"testing"

	gen "github.com/eth-easl/loader/pkg/generate"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	gen.InitSeed(42)
}

func TestGenCheckOverload(t *testing.T) {
	successCount := 100
	assert.False(t, gen.CheckOverload(int64(successCount), int64(successCount/10)))
	assert.True(t, gen.CheckOverload(int64(successCount), int64(3*successCount)))
}

func TestGenerateIat(t *testing.T) {
	t.Skip()

	/*totalNumInvocations := 30_000
	oneMinuteInMicro := 60_000_000.0
	halfMinuteInMicro := oneMinuteInMicro / 2*/

	/** Testing Equidistant */
	/*iats := gen.GenerateIAT(60, totalNumInvocations, gen.Equidistant)
	duration, _ := stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, iats[rand.Intn(len(iats))], iats[rand.Intn(len(iats))])
	assert.Equal(t, totalNumInvocations, len(iats))
	assert.Greater(t, oneMinuteInMicro, duration)
	t.Log("One-minute duration (equidistant): ", duration)

	for _, iat := range iats {
		assert.GreaterOrEqual(t, iat, 1.0)
	}*/

	/** Testing Exponential */
	/*iats = gen.GenerateIAT(60, totalNumInvocations, gen.Exponential)
	duration, _ = stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, totalNumInvocations, len(iats))
	assert.Greater(t, oneMinuteInMicro, duration)
	t.Log("One-minute duration (exponential): ", duration)

	for _, iat := range iats {
		assert.GreaterOrEqual(t, iat, 1.0)
	}

	iats = gen.GenerateIAT(60, 0, gen.Exponential)
	assert.Equal(t, 0, len(iats))*/
	// t.Log(iats)

	/** Testing shorter intervals. */
	/*iats = gen.GenerateIAT(30, totalNumInvocations, gen.Uniform)
	duration, _ = stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, totalNumInvocations, len(iats))
	assert.Greater(t, halfMinuteInMicro, duration)
	t.Log("Half-minute duration (uniform): ", duration)*/
}

func TestShuffling(t *testing.T) {
	arr := [][]int{}
	arr = append(arr, []int{1, 1, 2, 2, 3, 3})
	arr = append(arr, []int{1, 1, 2, 2, 3, 3})

	gen.ShuffleAllInvocationsInplace(&arr)

	isShuffled := false
	for idx := range arr {
		a := arr[idx]
		isShuffled = !sort.SliceIsSorted(a, func(i, j int) bool {
			return a[i] < a[j]
		})
		log.Info(arr)
	}
	assert.True(t, isShuffled)
}
