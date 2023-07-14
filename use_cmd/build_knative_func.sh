kn service delete gpt
sleep 2 

kn service create gpt \
--image gaow0007/trace_function_gpt_gpu:latest \
--port h2c:80 \
--limit cpu=2000m,memory=8000Mi


# kubectl get pods -o name | xargs -I {}  kubectl logs -f {} user-container

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