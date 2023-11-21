# TODO

- switch case relating function names(20 chars of the hash) with different images
- building images with different sizes of sqrt operations
- deploy the images to docker hub with own account
- make sure images are not pre-fetched in the cluster

- we need to create new `trace_func_go.yaml` for each new function, and use it as `yamlPath` in the deployment functions
- make a switch case for deciding the both yaml file to be used in the deployment and also the image that should be used, put this into the `DeployFunctions`, maybe use `sed` command for that

- fix git fetch in the node
- learn how to disable/enable some plugins in the kubernetes, maybe `kubectl` commands are useful