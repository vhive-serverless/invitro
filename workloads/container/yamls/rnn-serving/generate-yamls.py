import yaml

template_yaml = """
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: rnn-serving-python-{x}-{y}
  namespace: default
spec:
  template:
    metadata:
      annotations:
        autoscaling.knative.dev/initial-scale: "0"  # Should start from 0, otherwise we can't deploy more functions than the node physically permits.
        autoscaling.knative.dev/min-scale: "0"  # This parameter only has a per-revision key, so it's necessary to have here in case of the warmup messes up.
        autoscaling.knative.dev/target-burst-capacity: "-1"  # Put activator always in the path explicitly.
        autoscaling.knative.dev/max-scale: "200"  # Maximum instances limit of Azure.

        autoscaling.knative.dev/panic-window-percentage: $PANIC_WINDOW
        autoscaling.knative.dev/panic-threshold-percentage: $PANIC_THRESHOLD
        autoscaling.knative.dev/metric: $AUTOSCALING_METRIC
        autoscaling.knative.dev/target: $AUTOSCALING_TARGET
    spec:
      containerConcurrency: 1
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: loader-nodetype
                operator: In
                values:
                - worker
                - singlenode
      containers:
        - image: docker.io/vhiveease/relay:latest
          ports:
            - name: h2c
              containerPort: 50000
          args:
            - --addr=0.0.0.0:50000
            - --function-endpoint-url=0.0.0.0
            - --function-endpoint-port=50051
            - --function-name=rnn-serving-python
            - --value=French
            - --generator=random
            - --lowerBound={x}
            - --upperBound={y}
            - --profile-function=true
        - image: docker.io/vhiveease/rnn-serving-python:latest
          args:
            - --addr=0.0.0.0
            - --port=50051
            - --default_language=French
            - --num_strings=15
          env:
            - name: ITERATIONS_MULTIPLIER
              value: "102"
            - name: ENABLE_TRACING
              value: "false"
          resources:
            limits:
              cpu: $CPU_LIMITS
            requests:
              cpu: $CPU_REQUEST
              memory: $MEMORY_REQUESTS
"""

# List of x values
x_values = [1, 5, 10, 20, 45, 100, 200, 300, 450, 700, 1000, 1500, 2000, 2500, 3000, 4500, 7000, 8500, 10000, 12000, 14000, 16000, 18000, 20000]

# Generate YAML files for each combination of x and y values
for x in x_values:
    y = int(1.01 * x)
    yaml_content = template_yaml.format(x=x, y=y)
    filename = f"kn-rnn-serving-python-{x}-{y}.yaml"
    with open(filename, "w") as f:
        f.write(yaml_content)
    print(f"Created {filename}")
