kn service create gpt \
--image gaow0007/trace_function_gpt:latest \
--port h2c:80 

# docker.io/cvetkovic/trace_function:latest
# kn service create hello \
# --image ghcr.io/knative/helloworld-go:latest \
# --port 8080 \
# --env TARGET=World

# curl "$(kn service describe gpt -o url)"
# kn service delete gpt
# kubectl exec -it gpt-00001-deployment-6699f56d57-nnrmr -- /bin/sh
# kn service delete gpt ; bash demo_test.sh ; curl "$(kn service describe gpt -o url)"
# kubectl get pods -name 
# kubectl logs -f user-container 