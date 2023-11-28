from os import system

system("crictl --runtime-endpoint=unix:///var/run/containerd/containerd.sock --image-endpoint=unix:///var/run/containerd/containerd.sock images > imageList.txt")
image_list = []

with open("imageList.txt", "r") as f:
    for line in f:
        cur = line.strip().split()
        if cur[0].startswith("docker.io/lfavento"):
            image_list.append(cur[2])

image_list = " ".join(image_list)
system("crictl --runtime-endpoint=unix:///var/run/containerd/containerd.sock --image-endpoint=unix:///var/run/containerd/containerd.sock rmi " + image_list)