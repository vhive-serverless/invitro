for s in $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep -v activator);
do
  kubectl delete pod $s -n knative-serving &
done
for s in $(kubectl get pods -n kube-system -o name | cut -c 5-);
do
  kubectl delete pod $s -n kube-system &
done
