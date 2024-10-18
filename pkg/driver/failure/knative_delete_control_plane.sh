# autoscaler
kubectl delete pod $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep autoscaler | tail -n 1) -n knative-serving &

# controller
kubectl delete pod $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep controller | tail -n 1) -n knative-serving &

# net-istio-controller
kubectl delete pod $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep net-istio-controller | tail -n 1) -n knative-serving &

# net-istio-webhook
kubectl delete pod $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep net-istio-webhook | tail -n 1) -n knative-serving &

# webhook
kubectl delete pod $(kubectl get pods -n knative-serving -o name | cut -c 5- | grep webhook | tail -n 1) -n knative-serving &

# etcd-node
kubectl delete pod $(kubectl get pods -n kube-system -o name | cut -c 5- | grep etcd-node | tail -n 1) -n kube-system &

# kube-apiserver
kubectl delete pod $(kubectl get pods -n kube-system -o name | cut -c 5- | grep kube-apiserver | tail -n 1) -n kube-system &

# kube-controller-manager
kubectl delete pod $(kubectl get pods -n kube-system -o name | cut -c 5- | grep kube-controller-manager | tail -n 1) -n kube-system &

# kube-scheduler
kubectl delete pod $(kubectl get pods -n kube-system -o name | cut -c 5- | grep kube-scheduler | tail -n 1) -n kube-system &

# TODO: make an automatic way to choose leaders instead of picking a random one to kill