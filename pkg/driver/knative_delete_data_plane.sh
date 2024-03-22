for s in $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep activator);
do
  kubectl delete pod $s -n knative-serving &
done
for s in $(kubectl get pods -n istio-system -o name | cut -c 5- );
do
  kubectl delete pod $s -n istio-system &
done
