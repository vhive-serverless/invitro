package driver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/eth-easl/loader/pkg/common"
	invokefunc "github.com/eth-easl/loader/pkg/driver/invokefunc"
	mc "github.com/eth-easl/loader/pkg/metric"
	"github.com/eth-easl/loader/pkg/workload/schedproto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func (d *Driver) createElasticFlowSchedExecutor(filename string, jobschedrequest chan *mc.JobSchedRequest, jobschedreply chan *mc.JobSchedReply) {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		fmt.Println("Failed to connect to server:", err)
		return
	}
	red := "\033[31m"
	reset := "\033[0m"
	fmt.Println(red + "Starting create SchedExecutor" + reset)
	defer conn.Close()
	client := schedproto.NewExecutorClient(conn)
	var schedDone bool = false
	var curRequestCount int = 0
	for !schedDone {
		{
			requests := make([]*schedproto.SchedRequest, 0)
			curRequestCount = 0
			for {
				if curRequestCount == invokefunc.QueryJobInScheduleCount() {
					break
				} else {
					// message := fmt.Sprintf("curRequestCount == %d, %d", curRequestCount, invokefunc.QueryJobInScheduleCount())
					// fmt.Println(red + message + reset)
				}
				select {
				case request := <-jobschedrequest:
					// fmt.Println(red + "This text will be printed in red color!" + reset)
					curRequestCount++
					newSchedRequest := &schedproto.SchedRequest{
						SchedAlg:          d.Configuration.LoaderConfiguration.ClientTraining,
						InvocationName:    request.InvocationID,
						Batchsize:         request.BatchSize,
						RuntimeInMilliSec: request.RuntimeInMilliSec,
						Iterations:        request.Iterations,
						Deadline:          request.Deadline,
						PrevReplica:       request.PrevReplica,
						AvailableGPU:      request.AvailableGPU,
					}
					requests = append(requests, newSchedRequest)
				} // end of select
			}

			if curRequestCount > 0 {
				responseStream, err := client.ExecuteStream(context.Background())
				if err != nil {
					fmt.Println("Failed to execute stream request:", err)
					return
				}
				for _, request := range requests {
					err := responseStream.Send(request)
					if err != nil {
						fmt.Println("Failed to send request:", err)
						continue
					}
				}
				responseStream.CloseSend()
				response, err := responseStream.CloseAndRecv()
				for i := 0; i < len(requests); i++ {
					jobreply := &mc.JobSchedReply{
						InvocationIDs: response.InvocationName,
						Replicas:      response.Replica,
					}
					message := fmt.Sprintf("InvocationIDs : %v, Replicas: %v", jobreply.InvocationIDs, jobreply.Replicas)
					fmt.Println(red + message + reset)

					jobschedreply <- jobreply
					if err != nil {
						log.Fatalf("Failed to receive response: %v", err)
					}
					// message := fmt.Sprintf("i == %d, response == %v", i, response)
					// fmt.Println(red + message + reset)
				}
				curRequestCount = 0
			}
		}

		time.Sleep(time.Duration(10) * time.Millisecond)
		if QueryFinish() {
			close(jobschedreply)
			close(jobschedrequest)
			break
		}
	}
}

func (d *Driver) createINFlessSchedExecutor(filename string, jobschedrequest chan *mc.JobSchedRequest, jobschedreply chan *mc.JobSchedReply) {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		fmt.Println("Failed to connect to server:", err)
		return
	}
	red := "\033[31m"
	reset := "\033[0m"
	fmt.Println(red + "Starting create SchedExecutor" + reset)
	defer conn.Close()
	client := schedproto.NewExecutorClient(conn)
	var schedDone bool = false
	var curRequestCount int = 0
	for !schedDone {
		{
			requests := make([]*schedproto.SchedRequest, 0)
			curRequestCount = 0
			for {
				if curRequestCount == invokefunc.QueryJobInScheduleCount() {
					break
				} else {
					// message := fmt.Sprintf("curRequestCount == %d, %d", curRequestCount, invokefunc.QueryJobInScheduleCount())
					// fmt.Println(red + message + reset)
				}
				select {
				case request := <-jobschedrequest:
					// fmt.Println(red + "This text will be printed in red color!" + reset)
					curRequestCount++
					newSchedRequest := &schedproto.SchedRequest{
						SchedAlg:          d.Configuration.LoaderConfiguration.ClientTraining,
						InvocationName:    request.InvocationID,
						Batchsize:         request.BatchSize,
						RuntimeInMilliSec: request.RuntimeInMilliSec,
						Iterations:        request.Iterations,
						Deadline:          request.Deadline,
						PrevReplica:       request.PrevReplica,
						AvailableGPU:      int32(common.TotalGPUs - invokefunc.QueryResourceUsed()),
					}
					requests = append(requests, newSchedRequest)
				} // end of select
			}

			if curRequestCount > 0 {
				responseStream, err := client.ExecuteStream(context.Background())
				if err != nil {
					fmt.Println("Failed to execute stream request:", err)
					return
				}
				for _, request := range requests {
					err := responseStream.Send(request)
					if err != nil {
						fmt.Println("Failed to send request:", err)
						continue
					}
				}
				responseStream.CloseSend()
				response, err := responseStream.CloseAndRecv()
				for i := 0; i < len(requests); i++ {
					jobreply := &mc.JobSchedReply{
						InvocationIDs: response.InvocationName,
						Replicas:      response.Replica,
					}
					// message := fmt.Sprintf("InvocationIDs : %v, Replicas: %v", jobreply.InvocationIDs, jobreply.Replicas)
					// fmt.Println(red + message + reset)

					jobschedreply <- jobreply
					if err != nil {
						log.Fatalf("Failed to receive response: %v", err)
					}
					// message := fmt.Sprintf("i == %d, response == %v", i, response)
					// fmt.Println(red + message + reset)
				}
				curRequestCount = 0
			}
		}

		time.Sleep(time.Duration(10) * time.Millisecond)
		if QueryFinish() {
			close(jobschedreply)
			close(jobschedrequest)
			break
		}
	}
}

func (d *Driver) createElasticSchedExecutor(filename string, jobschedrequest chan *mc.JobSchedRequest, jobschedreply chan *mc.JobSchedReply) {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		fmt.Println("Failed to connect to server:", err)
		return
	}
	red := "\033[31m"
	reset := "\033[0m"
	fmt.Println(red + "Starting create SchedExecutor" + reset)
	defer conn.Close()
	client := schedproto.NewExecutorClient(conn)
	var schedDone bool = false
	var curRequestCount int = 0
	for !schedDone {
		{
			requests := make([]*schedproto.SchedRequest, 0)
			curRequestCount = 0
			for {
				if curRequestCount == invokefunc.QueryJobInScheduleCount() {
					break
				} else {
					message := fmt.Sprintf("curRequestCount == %d, %d", curRequestCount, invokefunc.QueryJobInScheduleCount())
					fmt.Println(red + message + reset)
				}
				select {
				case request := <-jobschedrequest:
					// fmt.Println(red + "This text will be printed in red color!" + reset)
					curRequestCount++
					newSchedRequest := &schedproto.SchedRequest{
						SchedAlg:          d.Configuration.LoaderConfiguration.ClientTraining,
						InvocationName:    request.InvocationID,
						Batchsize:         request.BatchSize,
						RuntimeInMilliSec: request.RuntimeInMilliSec,
						Iterations:        request.Iterations,
						Deadline:          request.Deadline,
						PrevReplica:       request.PrevReplica,
						AvailableGPU:      int32(common.TotalGPUs - invokefunc.QueryResourceUsed()),
					}
					requests = append(requests, newSchedRequest)
				} // end of select
			}

			if curRequestCount > 0 {
				responseStream, err := client.ExecuteStream(context.Background())
				if err != nil {
					fmt.Println("Failed to execute stream request:", err)
					return
				}
				for _, request := range requests {
					err := responseStream.Send(request)
					if err != nil {
						fmt.Println("Failed to send request:", err)
						continue
					}
				}
				responseStream.CloseSend()
				response, err := responseStream.CloseAndRecv()
				for i := 0; i < len(requests); i++ {
					jobreply := &mc.JobSchedReply{
						InvocationIDs: response.InvocationName,
						Replicas:      response.Replica,
					}
					message := fmt.Sprintf("InvocationIDs : %v, Replicas: %v", jobreply.InvocationIDs, jobreply.Replicas)
					fmt.Println(red + message + reset)

					jobschedreply <- jobreply
					if err != nil {
						log.Fatalf("Failed to receive response: %v", err)
					}
					// message := fmt.Sprintf("i == %d, response == %v", i, response)
					// fmt.Println(red + message + reset)
				}
				curRequestCount = 0
			}
		}

		time.Sleep(time.Duration(10) * time.Millisecond)
		if QueryFinish() {
			close(jobschedreply)
			close(jobschedrequest)
			break
		}
	}
}

func (d *Driver) startSchedBackgroundProcesses(allRecordsWritten *sync.WaitGroup) (chan *mc.JobSchedRequest, chan *mc.JobSchedReply) {
	jobSchedRequest := make(chan *mc.JobSchedRequest)
	jobSchedReply := make(chan *mc.JobSchedReply)
	if IsStringInList(d.Configuration.LoaderConfiguration.ClientTraining, []string{common.ElasticFlow}) {
		go d.createElasticFlowSchedExecutor(d.outputFilename("sched"), jobSchedRequest, jobSchedReply)
	} else if IsStringInList(d.Configuration.LoaderConfiguration.ClientTraining, []string{common.INFless}) {
		go d.createINFlessSchedExecutor(d.outputFilename("sched"), jobSchedRequest, jobSchedReply)
	} else if IsStringInList(d.Configuration.LoaderConfiguration.ClientTraining, []string{common.Elastic}) {
		go d.createElasticSchedExecutor(d.outputFilename("sched"), jobSchedRequest, jobSchedReply)
	}

	return jobSchedRequest, jobSchedReply
}
