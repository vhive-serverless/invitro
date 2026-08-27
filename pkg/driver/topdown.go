package driver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	errMu          sync.Mutex
	errs           []error
}

var (
	serverExec          = utils.ServerExec
	perfCollectionSleep = time.Sleep
)

func (p *PerfCollectionContext) addError(err error) {
	if err == nil {
		return
	}
	p.errMu.Lock()
	p.errs = append(p.errs, err)
	p.errMu.Unlock()
}

func (p *PerfCollectionContext) collectionError() error {
	p.errMu.Lock()
	defer p.errMu.Unlock()
	if len(p.errs) == 0 {
		return nil
	}
	parts := make([]string, len(p.errs))
	for i, err := range p.errs {
		parts[i] = err.Error()
	}
	return fmt.Errorf("perf collection failed: %s", strings.Join(parts, "; "))
}

func perfOutputPrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("perf output prefix is empty")
	}
	return filepath.Abs(prefix)
}

func perfArtifactPaths(prefix string, node int) []string {
	return []string{fmt.Sprintf("%s_perf_%d.csv", prefix, node), fmt.Sprintf("%s_perf_%d.data", prefix, node), fmt.Sprintf("%s_perf_%d.svg", prefix, node), fmt.Sprintf("%s_perf_filtered_%d.svg", prefix, node)}
}

func StartPerfCollection(cfg config.Configuration, ctx context.Context) (*PerfCollectionContext, error) {
	if cfg.LoaderConfiguration == nil {
		return nil, fmt.Errorf("loader configuration is nil")
	}
	prefix, err := perfOutputPrefix(cfg.LoaderConfiguration.OutputPathPrefix)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(prefix), 0755); err != nil {
		return nil, fmt.Errorf("create perf output directory: %w", err)
	}
	waitTime := cfg.LoaderConfiguration.WarmupDuration * 60         // in seconds
	perfStatTime := cfg.LoaderConfiguration.ExperimentDuration * 60 // in seconds
	perfStatTimeInMs := perfStatTime * 1000

	var workerNodeIps []string
	workerNodeIpRaw, err := exec.Command("bash", "-c", `kubectl get nodes -o wide -l 'loader-nodetype in (worker, singlenode)' | awk 'NR>1 {print $6}'`).Output()
	if err != nil {
		return nil, fmt.Errorf("retrieve worker node ip: %w", err)
	}
	workerNodeIps = strings.Fields(string(workerNodeIpRaw))
	if len(workerNodeIps) == 0 {
		return nil, fmt.Errorf("retrieve worker node ip: no labeled worker node found")
	}

	loaderNodeIpRaw, err := exec.Command("sh", "-c", `ip addr show | awk '/inet 10\.0\.1\./{split($2, a, "/"); print a[1]}'`).Output()
	if err != nil {
		return nil, fmt.Errorf("retrieve experiment ip: %w", err)
	}
	loaderNodeIp := strings.TrimSpace(string(loaderNodeIpRaw))
	if loaderNodeIp == "" {
		return nil, fmt.Errorf("retrieve experiment ip: no 10.0.1.x address found")
	}

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
	BASELINE_Hk := addModifiers(BASELINE, "Hk")
	BASELINE_Gk := addModifiers(BASELINE, "Gk")
	BASELINE_Hu := addModifiers(BASELINE, "Hu")
	BASELINE_Gu := addModifiers(BASELINE, "Gu")

	commandList := []string{
		fmt.Sprintf("-e %s,%s,%s,%s", BASELINE_H, TMA_H, CACHE_EVENTS_H, TLB_MISSES_H),                 // Multiplexing 36%
		fmt.Sprintf("-e %s,%s,%s,%s,%s", BASELINE_G, TMA_G, CACHE_EVENTS_G, TLB_MISSES_G, MISC_EVENTS), // Multiplexing 36%
		fmt.Sprintf("-e %s,%s,%s,%s", BASELINE_Hk, BASELINE_Gk, BASELINE_Hu, BASELINE_Gu),
	}

	perPerfTime := int((float64(perfStatTimeInMs) * 0.8) / float64(len(commandList)+1)) // 1 for wait, and one for recording
	// waitTime += int(perPerfTime / 1000)

	loaderCfg := *cfg.LoaderConfiguration
	loaderCfg.OutputPathPrefix = prefix
	cfg.LoaderConfiguration = &loaderCfg
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
		if _, err := serverExec(node, "rm -f ~/perf.csv ~/perf.data ~/perf.svg ~/perf_filtered.svg"); err != nil {
			return nil, fmt.Errorf("clear stale perf outputs on node %s: %w", node, err)
		}
		perfCtx.cancelChannels[nodeIndex] = make(chan struct{})
		perfCtx.wg.Add(1)
		go func(node string, nodeIdx int, cancelCh chan struct{}) {
			defer perfCtx.wg.Done()

			// Wait for warmup period
			perfCollectionSleep(time.Duration(waitTime) * time.Second)
			log.Debugf("Starting perf collection on node %s (index %d)", node, nodeIdx)

			// Start perf stat commands
			for _, command := range commandList {
				perfCommand := fmt.Sprintf("sudo perf stat %s -a --no-csv-summary --timeout %d -x, -o ~/perf.csv --append", command, perPerfTime)
				log.Debugf("Running perf command on node %s: %s", node, perfCommand)
				_, err := serverExec(node, perfCommand)
				if err != nil {
					perfCtx.addError(fmt.Errorf("perf stat on node %s: %w", node, err))
					return
				}

				// Check if we should cancel
				select {
				case <-cancelCh:
					log.Debugf("Perf collection cancelled on node %s", node)
					return
				default:
				}
			}

			// Perf record

			perfRecordCommand := fmt.Sprintf("sudo perf kvm --host --guest record -e cycles,faults -F 199 -ag -o ~/perf.data sleep %d", perPerfTime/1000)
			log.Debugf("Running perf record command on node %s: %s", node, perfRecordCommand)
			_, err := serverExec(node, perfRecordCommand)
			if err != nil {
				perfCtx.addError(fmt.Errorf("perf record on node %s: %w", node, err))
				return
			}

			log.Debugf("Perf collection completed on node %s (index %d)", node, nodeIdx)
		}(node, nodeIndex, perfCtx.cancelChannels[nodeIndex])
	}

	return perfCtx, nil
}

func StopPerfCollection(perfCtx *PerfCollectionContext) error {
	if perfCtx == nil {
		log.Warn("No perf collection context to stop")
		return nil
	}

	log.Info("Stopping perf collection and collecting results...")

	for _, cancelCh := range perfCtx.cancelChannels {
		close(cancelCh)
	}

	perfCtx.wg.Wait()

	perfCollectionSleep(2 * time.Second)

	// Now rsync the results back
	rsyncWg := sync.WaitGroup{}
	for nodeIndex, node := range perfCtx.workerNodeIps {
		rsyncWg.Add(1)
		go func(node string, nodeIdx int) {
			defer rsyncWg.Done()
			nodeFailed := false

			// Rsync the perf.csv file back to loader node
			rsyncCommand := fmt.Sprintf("rsync -avz -e ssh ~/perf.csv %s:%s_perf_%d.csv",
				perfCtx.loaderNodeIp, perfCtx.cfg.LoaderConfiguration.OutputPathPrefix, nodeIdx)
			log.Debugf("Collecting perf results from node %s: %s", node, rsyncCommand)
			_, err := serverExec(node, rsyncCommand)
			if err != nil {
				nodeFailed = true
				perfCtx.addError(fmt.Errorf("rsync perf csv on node %s: %w", node, err))
			}
			// Fix file permissions so the standard user can rsync it
			chownCommand := "sudo chown $USER:$(id -gn) ~/perf.data" //need to change user to group
			log.Debugf("Fixing permissions on node %s: %s", node, chownCommand)
			_, err = serverExec(node, chownCommand)
			if err != nil {
				nodeFailed = true
				perfCtx.addError(fmt.Errorf("chown perf data on node %s: %w", node, err))
			}
			// Rsync the perf.data file back to loader node
			rsyncCommand = fmt.Sprintf("rsync -avz -e ssh ~/perf.data %s:%s_perf_%d.data",
				perfCtx.loaderNodeIp, perfCtx.cfg.LoaderConfiguration.OutputPathPrefix, nodeIdx)
			log.Debugf("Collecting perf results from node %s: %s", node, rsyncCommand)
			_, err = serverExec(node, rsyncCommand)
			if err != nil {
				nodeFailed = true
				perfCtx.addError(fmt.Errorf("rsync perf data on node %s: %w", node, err))
			}

			guestVmlinuxPath := "/users/$USER/khala/assets/vmlinux-shmem/vmlinux"
			perfSCriptCmd := fmt.Sprintf("set -o pipefail; sudo perf script --kallsyms=/proc/kallsyms --guestvmlinux=%s -i ~/perf.data -f | ~/FlameGraph/stackcollapse-perf.pl --event-filter=cycles | sed -E 's/^:[0-9]+/fc_kvm_exec/' > data.folded-base && ~/FlameGraph/flamegraph.pl data.folded-base > ~/perf.svg", guestVmlinuxPath)
			log.Debugf("Collecting perf stacks from node %s: %s", node, perfSCriptCmd)
			_, err = serverExec(node, perfSCriptCmd)
			if err != nil {
				nodeFailed = true
				perfCtx.addError(fmt.Errorf("postprocess perf stacks on node %s: %w", node, err))
			}
			rsyncCommand = fmt.Sprintf("rsync -avz -e ssh ~/perf.svg %s:%s_perf_%d.svg",
				perfCtx.loaderNodeIp, perfCtx.cfg.LoaderConfiguration.OutputPathPrefix, nodeIdx)
			log.Debugf("Collecting perf stacks from node %s: %s", node, rsyncCommand)
			_, err = serverExec(node, rsyncCommand)
			if err != nil {
				nodeFailed = true
				perfCtx.addError(fmt.Errorf("rsync perf svg on node %s: %w", node, err))
			}
			perfSCriptFilteredCmd := fmt.Sprintf("set -o pipefail; sudo perf script --kallsyms=/proc/kallsyms --guestvmlinux=%s -i ~/perf.data -f | ~/FlameGraph/stackcollapse-perf.pl --event-filter=cycles | sed -E 's/^:[0-9]+/fc_kvm_exec/' > data.folded-base && grep -e \"firecracker\" -e \"nexus-backend\" -e \"fc_vcpu\" -e \"fc_kvm_exec\" data.folded-base | ~/FlameGraph/flamegraph.pl > ~/perf_filtered.svg", guestVmlinuxPath)
			log.Debugf("Collecting perf stacks from node %s: %s", node, perfSCriptFilteredCmd)
			_, err = serverExec(node, perfSCriptFilteredCmd)
			if err != nil {
				nodeFailed = true
				perfCtx.addError(fmt.Errorf("postprocess filtered perf stacks on node %s: %w", node, err))
			}
			rsyncCommand = fmt.Sprintf("rsync -avz -e ssh ~/perf_filtered.svg %s:%s_perf_filtered_%d.svg",
				perfCtx.loaderNodeIp, perfCtx.cfg.LoaderConfiguration.OutputPathPrefix, nodeIdx)
			log.Debugf("Collecting perf stacks from node %s: %s", node, rsyncCommand)
			_, err = serverExec(node, rsyncCommand)
			if err != nil {
				nodeFailed = true
				perfCtx.addError(fmt.Errorf("rsync filtered perf svg on node %s: %w", node, err))
			}
			for _, artifact := range perfArtifactPaths(perfCtx.cfg.LoaderConfiguration.OutputPathPrefix, nodeIdx) {
				info, statErr := os.Stat(artifact)
				if statErr != nil || info.Size() == 0 {
					nodeFailed = true
					perfCtx.addError(fmt.Errorf("missing or empty perf artifact %s", artifact))
				}
			}
			if nodeFailed {
				log.Errorf("Preserving worker perf files on node %s after collection failure", node)
				return
			}
			cleanupCommand := "sudo rm -f ~/perf.csv ~/perf.data ~/perf.svg ~/perf_filtered.svg ~/data.folded-base"
			log.Debugf("Cleaning up copied perf files on node %s", node)
			if _, err = serverExec(node, cleanupCommand); err != nil {
				perfCtx.addError(fmt.Errorf("cleanup copied perf files on node %s: %w", node, err))
			}
		}(node, nodeIndex)
	}

	rsyncWg.Wait()
	if err := perfCtx.collectionError(); err != nil {
		return err
	}
	log.Info("Perf collection results have been collected successfully")
	return nil
}
