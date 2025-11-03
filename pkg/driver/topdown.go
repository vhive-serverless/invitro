package driver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/scripts/setup/utils"
)

func addModifiers(events string, modifier string) string {
	eventsList := strings.Split(events, ",")
	for i, event := range eventsList {
		eventsList[i] = fmt.Sprintf("%s:%s", event, modifier)
	}
	return strings.Join(eventsList, ",")
}

type PerfCollectionContext struct {
	cfg            config.Configuration
	workerNodeIps  []string
	loaderNodeIp   string
	commandList    []string
	perPerfTime    int
	cancelChannels []chan struct{}
	wg             sync.WaitGroup
}

func StartPerfCollection(cfg config.Configuration, ctx context.Context) *PerfCollectionContext {
	waitTime := cfg.LoaderConfiguration.WarmupDuration * 60         // in seconds
	perfStatTime := cfg.LoaderConfiguration.ExperimentDuration * 60 // in seconds
	perfStatTimeInMs := perfStatTime * 1000

	var workerNodeIps []string
	workerNodeIpRaw, err := exec.Command("bash", "-c", `kubectl get nodes -o wide -l 'loader-nodetype in (worker, singlenode)' | awk 'NR>1 {print $6}'`).Output()
	if err != nil {
		log.Fatal("failed to retrieve worker node ip")
	}
	workerNodeIps = append(workerNodeIps, strings.Split(strings.TrimSpace(string(workerNodeIpRaw)), "\n")...)

	loaderNodeIpRaw, err := exec.Command("sh", "-c", `ip addr show | awk '/inet 10\.0\.1\./{split($2, a, "/"); print a[1]}'`).Output()
	if err != nil {
		log.Fatal("failed to retrieve experiment ip")
	}
	loaderNodeIp := strings.TrimSpace(string(loaderNodeIpRaw))

	BASELINE := "instructions,cpu-cycles"
	TMA := "IDQ_UOPS_NOT_DELIVERED.CORE,INT_MISC.UOP_DROPPING,TOPDOWN.SLOTS_P,TOPDOWN.BACKEND_BOUND_SLOTS,UOPS_RETIRED.SLOTS,TOPDOWN.MEMORY_BOUND_SLOTS,IDQ_BUBBLES.CYCLES_0_UOPS_DELIV.CORE,TOPDOWN.BR_MISPREDICT_SLOTS,BR_MISP_RETIRED.ALL_BRANCHES,BR_INST_RETIRED.ALL_BRANCHES"
	CACHE_EVENTS := "L1-icache-load-misses,L1D.REPLACEMENT,L2_RQSTS.ALL_CODE_RD,L2_LINES_IN.ALL,MEM_LOAD_RETIRED.L2_MISS,L2_RQSTS.CODE_RD_MISS,LLC-load-misses,LLC-store-misses"
	TLB_EVENTS := "ITLB_MISSES.WALK_COMPLETED,DTLB_LOAD_MISSES.WALK_COMPLETED,DTLB_STORE_MISSES.WALK_COMPLETED"
	MISC_EVENTS := "kvm:kvm_exit,kvm:kvm_vcpu_wakeup,kvm:kvm_mmio,kvm:kvm_pio,kvm:kvm_hypercall,kvm:kvm_inj_virq,kvm:kvm_set_irq,context-switches,page-faults"

	BASELINE_H := addModifiers(BASELINE, "H")
	BASELINE_G := addModifiers(BASELINE, "G")
	TMA_H := addModifiers(TMA, "H")
	TMA_G := addModifiers(TMA, "G")
	CACHE_EVENTS_H := addModifiers(CACHE_EVENTS, "H")
	CACHE_EVENTS_G := addModifiers(CACHE_EVENTS, "G")
	TLB_MISSES_H := addModifiers(TLB_EVENTS, "H")
	TLB_MISSES_G := addModifiers(TLB_EVENTS, "G")

	// BASELINE_Hk := addModifiers(BASELINE, "Hk")
	// BASELINE_Gk := addModifiers(BASELINE, "Gk")
	// BASELINE_Hu := addModifiers(BASELINE, "Hu")
	// BASELINE_Gu := addModifiers(BASELINE, "Gu")

	// commandList := []string{
	// 	fmt.Sprintf("-e %s,%s,%s,%s", BASELINE_H, TMA_H, CACHE_EVENTS_H, TLB_MISSES_H),                 // Multiplexing 36%
	// 	fmt.Sprintf("-e %s,%s,%s,%s,%s", BASELINE_G, TMA_G, CACHE_EVENTS_G, TLB_MISSES_G, MISC_EVENTS), // Multiplexing 36%
	// }

	commandList := []string{
		fmt.Sprintf("-e %s,%s,%s,%s", BASELINE_H, TMA_H, CACHE_EVENTS_H, TLB_MISSES_H),                 // Multiplexing 36%
		fmt.Sprintf("-e %s,%s,%s,%s,%s", BASELINE_G, TMA_G, CACHE_EVENTS_G, TLB_MISSES_G, MISC_EVENTS), // Multiplexing 36%
	}

	perPerfTime := int((float64(perfStatTimeInMs) * 0.8) / float64(len(commandList)+1))
	waitTime += int(perPerfTime / 1000)

	perfCtx := &PerfCollectionContext{
		cfg:            cfg,
		workerNodeIps:  workerNodeIps,
		loaderNodeIp:   loaderNodeIp,
		commandList:    commandList,
		perPerfTime:    perPerfTime,
		cancelChannels: make([]chan struct{}, len(workerNodeIps)),
	}

	log.Info("Starting perf collection on worker nodes...")

	for nodeIndex, node := range workerNodeIps {
		perfCtx.cancelChannels[nodeIndex] = make(chan struct{})
		perfCtx.wg.Add(1)
		go func(node string, nodeIdx int, cancelCh chan struct{}) {
			defer perfCtx.wg.Done()

			// Wait for warmup period
			time.Sleep(time.Duration(waitTime) * time.Second)
			log.Debugf("Starting perf collection on node %s (index %d)", node, nodeIdx)

			// Start perf stat commands
			for _, command := range commandList {
				perfCommand := fmt.Sprintf("sudo perf stat %s -a --no-csv-summary --timeout %d -x, -o ~/perf.csv --append", command, perPerfTime)
				log.Debugf("Running perf command on node %s: %s", node, perfCommand)
				_, err := utils.ServerExec(node, perfCommand)
				if err != nil {
					log.Errorf("Failed to run perf command on node %s: %v", node, err)
				}

				// Check if we should cancel
				select {
				case <-cancelCh:
					log.Debugf("Perf collection cancelled on node %s", node)
					return
				default:
				}
			}

			log.Debugf("Perf collection completed on node %s (index %d)", node, nodeIdx)
		}(node, nodeIndex, perfCtx.cancelChannels[nodeIndex])
	}

	return perfCtx
}

func StopPerfCollection(perfCtx *PerfCollectionContext) {
	if perfCtx == nil {
		log.Warn("No perf collection context to stop")
		return
	}

	log.Info("Stopping perf collection and collecting results...")

	for _, cancelCh := range perfCtx.cancelChannels {
		close(cancelCh)
	}

	perfCtx.wg.Wait()

	time.Sleep(2 * time.Second)

	// Now rsync the results back
	rsyncWg := sync.WaitGroup{}
	for nodeIndex, node := range perfCtx.workerNodeIps {
		rsyncWg.Add(1)
		go func(node string, nodeIdx int) {
			defer rsyncWg.Done()

			// Rsync the perf.csv file back to loader node
			rsyncCommand := fmt.Sprintf("rsync -avz -e ssh ~/perf.csv %s:~/loader/%s_perf_%d.csv",
				perfCtx.loaderNodeIp, perfCtx.cfg.LoaderConfiguration.OutputPathPrefix, nodeIdx)
			log.Debugf("Collecting perf results from node %s: %s", node, rsyncCommand)
			_, err := utils.ServerExec(node, rsyncCommand)
			if err != nil {
				log.Errorf("Failed to rsync perf results from node %s: %v", node, err)
			}

			// Clean up the remote perf.csv file
			cleanupCommand := "sudo rm ~/perf.csv"
			log.Debugf("Cleaning up perf.csv on node %s", node)
			_, err = utils.ServerExec(node, cleanupCommand)
			if err != nil {
				log.Errorf("Failed to cleanup perf.csv on node %s: %v", node, err)
			}
		}(node, nodeIndex)
	}

	rsyncWg.Wait()
	log.Info("Perf collection results have been collected successfully")
}
