worker_list=("10.0.1.2" "10.0.1.3" "10.0.1.4" "10.0.1.5" "10.0.1.6")

for i in "${worker_list[@]}"; do
    echo "Cloning repo to $i"
    rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh "$i":~/ &
    rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.gitconfig "$i":~/ &
done
wait

for i in "${worker_list[@]}"; do
    echo "Cloning repo to $i"
    rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/khala "$i":~/ &
done
wait

for i in "${worker_list[@]}"; do
    echo "Setting up $i"
    ssh -oStrictHostKeyChecking=no "$i" "cd khala && bash scripts/setup_knative.sh" &
done
wait

for i in "${worker_list[@]}"; do
    echo "Building $i"
    ssh -oStrictHostKeyChecking=no "$i" "source /etc/profile && cd khala && make build-all" &
done
wait

for i in "${worker_list[@]}"; do
    echo "Make Jailer Directory $i"
    ssh -oStrictHostKeyChecking=no "$i" "sudo mkdir -p /mnt/resources/jailer" &
done
wait

echo "run network_tunneling script between master and workers"
