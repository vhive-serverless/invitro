kubectl rollout restart deployment/activator -n knative-serving 
kubectl rollout restart deployment/autoscaler -n knative-serving 
kubectl rollout restart deployment/controller -n knative-serving 
kubectl rollout restart deployment/webhook -n knative-serving 
