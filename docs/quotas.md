# Kubernetes and knative requests and limits

## Setting requests in k8s

Requests and limits in k8s are set for each container in pod as such (example from [Kubernetes docs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/)):

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: frontend
spec:
  containers:
  - name: app
    image: images.my-company.example/app:v4
    resources:
      requests: # request 64Mi and 250 mCPU for the 1st container
        memory: "64Mi"
        cpu: "250m"
      limits:   # limit the 1st container to 128Mi and 0.5 CPU
        memory: "128Mi"
        cpu: "500m"
  - name: log-aggregator
    image: images.my-company.example/log-aggregator:v6
    resources:
      requests: # request 128Mi and 125 mCPU for the 2nd container
        memory: "128Mi"
        cpu: "125m"
      limits:   # limit the 2st container to 256Mi and 250 mCPU
        memory: "256Mi"
        cpu: "250m"
```

During pod placement, the sum of requested resources are used as the pod quota that should be available on the node in order for the pod to be placed there. In the mentioned example, `192Mi` and `375 mCPU`.

Limits and requests are imposed per container using `cgroups`:

* CPU limit is the maximum share of CPU that a container can receive during CFS (Complete Fair Scheduling, Linux scheduling system)
* CPU request is the minimum amount of CPU that would be available to a pod and as a scheduling weight of the container when contention for CPU happens
* Memory limit is the hard limit of a container's memory. When the container tries to use more, the container runtime takes actions (commonly, the OOMKiller terminates the container)
* Memory request is the minimum amount of memory that will be available to a container no matter what

### Possible combinations of setting the CPU request and limit

|               | request set                                                                                            | request not set                                                                             |
|---------------|--------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| limit set     | container will get CPU between request and limit, CPU resource above total limit on the node is wasted | container will get exactly the requested amount due to implicit setting of request to limit |
| limit not set | request amount is guaranteed, excess CPU is utilized when needed                                       | no one is guaranteed anything                                                               |

In the case of setting the limit without a request, k8s implicitly sets the request to the limit, so in order not to request any resources but have a cap on resource utilization, a request of 0 CPU should be used.

CPU resource requested by a currently idling container may be used by a running container but without surpassing the CPU limit.

More considerations about combinations of CPU requests and can be found [here](https://home.robusta.dev/blog/stop-using-cpu-limits).

### Memory requests and limits

|               | request set                                                              | request not set                                                                                           |
|---------------|--------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------|
| limit set     | container will get memory between request and limit                      | container will be guaranteed the requested amount and no more due to implicit setting of request to limit |
| limit not set | request amount is guaranteed, free memory might be captured by container | no one is guaranteed anything                                                                             |

For a description of the case with limits and no requests, see the previous section about CPU.

Memory resource requested but unused by one container may be used by another container. But this may lead to the OOM killing the latter when the former decides to grow its memory usage inside the requested quota if memory is depleted on the node.

However, currently, serverless functions use significantly less memory than it requests. In order to increase memory utilization of the node, requests are commonly overcommitted.

More considerations about combinations of memory requests and can be found [here](https://home.robusta.dev/blog/kubernetes-memory-limit).

## Requests and knative

In knative requests and limits are set per function during its deployment. These numbers are used as requests and limits for `user-container` in function pods created by Kubernetes. For example (from [Knative docs](https://knative.dev/docs/serving/services/configure-requests-limits-services/)):

```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: example-service
  namespace: default
spec:
  template:
    spec:
      containers:
        - image: docker.io/user/example-app
          resources:
            requests:
              cpu: 100m
              memory: 640M
            limits:
              cpu: 1

```

Here, `100 mCPU` and `640M` will be reserved for `user-container` inside every instance of the function, and will be limited by `1 CPU`.

### Queue proxy requests

Function pods are supplied with a queue proxy sidecar. By default, the queue proxy is created with a CPU request of 25 mCPU, which might significantly limit the deployed pod density if `user-container` has a small CPU request by itself.

## Provider's view on requests and limits

Some providers bill clients based on different provisioned resources (e.g. [Google](https://cloud.google.com/functions/pricing#compute_time), [Alibaba](https://www.alibabacloud.com/help/en/function-compute/latest/instance-types-and-instance-modes#section-mfv-5fb-ehw)). These resource quotas can be imposed with setting user process' CPU and memory limits if deployed with Kubernetes.

Since utilization of provisioned CPU and memory in serverless services are low (see [RunD](https://www.usenix.org/conference/atc22/presentation/li-zijun-rund), Owl [SoCC '22] papers), serverless service providers use overcommitment of resources in order to increase resource utilization. However, this may lead to performance degradation (especially, tail latency) if more than expected number of invocations arrive at the same time.

## Usage in loader

Memory usage of a function is derived from the application's memory distribution by dividing it by the number of functions in the application to create a per-function distribution. Memory quota is considered to be the maximum memory usage of the function's invocations. Memory limit is not set. Memory request is overcommitted (by default, one tenth of derived memory quota).

Currently, no real memory usage is modeled by the function image due to the high amount of timeouts caused by slow `malloc`. As a result, the memory utilization of the node is modelled by the total memory requested by deployed pods. **We need to rethink this part, real world scenarios might be missed (OOM killer, overprovisioning, overcommitment, contention)**

CPU quotas are derived from the memory quotas, according to [GCP table](https://cloud.google.com/functions/pricing#compute_time) for quotas. CPU limit is set to that number, CPU request is overcommitted (by default, divided by ten).
