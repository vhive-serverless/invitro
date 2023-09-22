kubectl set image deployment/activator activator=docker.io/gaow0007/activator-ecd51ca5034883acbe737fde417a3d86:latest -n knative-serving
kubectl set image deployment/autoscaler autoscaler=docker.io/gaow0007/autoscaler-12c0fa24db31956a7cfa673210e4fa13:latest -n knative-serving
kubectl set image deployment/controller controller=docker.io/gaow0007/controller-f6fdb41c6acbc726e29a3104ff2ef720:latest -n knative-serving
kubectl set image deployment/webhook webhook=docker.io/gaow0007/webhook-261c6506fca17bc41be50b3461f98f1c:latest -n knative-serving

kubectl edit deployment activator -n knative-serving 
# gcr.io/knative-releases/knative.dev/serving/cmd/activator@sha256:624ad8df549de9192e7b0d2f596c08c5996f678b25ff94688464412890126bb1
kubectl edit deployment autoscaler -n knative-serving 
# gcr.io/knative-releases/knative.dev/serving/cmd/autoscaler@sha256:6b77fce9159173ce700cbe8202eb0ab33b2128688baadf34e916b7341889bde0
kubectl edit deployment controller -n knative-serving 
# gcr.io/knative-releases/knative.dev/serving/cmd/controller@sha256:712ac88a1ec328ad366a0245882061de01bdf631cb21bac2b621af8e20e7082d
kubectl edit deployment webhook -n knative-serving 
# gcr.io/knative-releases/knative.dev/serving/cmd/webhook@sha256:881a707c3f5b8537adf1fd460405fb8e65ae2676e4591926f9cdfb4454efd350

kubectl rollout restart deployment/activator -n knative-serving 
kubectl rollout restart deployment/autoscaler -n knative-serving 
kubectl rollout restart deployment/controller -n knative-serving 
kubectl rollout restart deployment/webhook -n knative-serving 

# check activator logs 
kubectl get pods -n knative-serving  | grep activator | awk '{print $1}' | xargs kubectl logs -f -n knative-serving 
kn service list  | awk '{print $1}' | xargs kn service delete 