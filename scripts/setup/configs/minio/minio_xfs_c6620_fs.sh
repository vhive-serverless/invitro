#!/bin/bash
# Exit immediately if a command exits with a non-zero status.
set -e

# --- Configuration ---
MOUNT_POINT="/mnt/resources/minio"
FILESYSTEM="xfs"
CANDIDATE_DEVICES=("/dev/nvme0n1" "/dev/nvme1n1")
TARGET_DEVICE=""

# --- Logic to find the target device ---
echo "INFO: Identifying the root filesystem's parent disk..."

# 1. Get the partition mounted at root '/' (e.g., /dev/nvme0n1p3)
ROOT_PARTITION=$(findmnt -n -o SOURCE /)

# 2. Get the parent disk of that partition (e.g., nvme0n1 from /dev/nvme0n1p3)
#    We add /dev/ to the front to get the full path (e.g., /dev/nvme0n1)
ROOT_DEVICE="/dev/$(lsblk -no pkname "${ROOT_PARTITION}")"

echo "INFO: Root filesystem is on ${ROOT_DEVICE}. This disk will be avoided."

# 3. Find the candidate device that is NOT the root device
for d in "${CANDIDATE_DEVICES[@]}"; do
    if [[ "${d}" != "${ROOT_DEVICE}" ]]; then
        TARGET_DEVICE="${d}"
        break
    fi
done

# 4. Final validation: Ensure a target was found and that it's not mounted
if [ -z "${TARGET_DEVICE}" ]; then
    echo "ERROR: Could not determine a target device to format."
    exit 1
fi

if findmnt "${TARGET_DEVICE}" &>/dev/null; then
    echo "ERROR: Safety check failed! The intended target ${TARGET_DEVICE} appears to be mounted."
    lsblk
    exit 1
fi

echo "================================================="
echo "INFO: Target device for formatting: ${TARGET_DEVICE}"
echo "================================================="


# --- Formatting and Mounting ---

echo "INFO: Formatting ${TARGET_DEVICE} with ${FILESYSTEM}..."
# The -f flag forces formatting if an old, unused filesystem exists
sudo mkfs."${FILESYSTEM}" -f "${TARGET_DEVICE}"

echo "INFO: Creating mount point ${MOUNT_POINT}..."
sudo mkdir -p "${MOUNT_POINT}"
sudo chmod 777 "${MOUNT_POINT}"

echo "INFO: Adding entry to /etc/fstab..."
UUID=$(sudo blkid -s UUID -o value "${TARGET_DEVICE}")

# Check if an entry for this UUID already exists to prevent duplicates
if grep -q "UUID=${UUID}" /etc/fstab; then
    echo "WARN: fstab entry for UUID=${UUID} already exists. Skipping add."
else
    # Backup fstab before modifying it, just in case.
    sudo cp /etc/fstab /etc/fstab.bak."$(date +%F-%T)"
    echo "INFO: Backed up /etc/fstab to /etc/fstab.bak.$(date +%F-%T)"
    
    # Append the new entry
    {
        echo "# Entry for ${TARGET_DEVICE} (${MOUNT_POINT}) added on $(date)"
        echo "UUID=${UUID}  ${MOUNT_POINT}  ${FILESYSTEM}  defaults,nofail  0  2"
    } | sudo tee -a /etc/fstab > /dev/null
    echo "INFO: Successfully added fstab entry."
fi

echo "INFO: Mounting all filesystems defined in /etc/fstab..."
sudo mount -a

echo "INFO: Reloading systemd manager configuration..."
sudo systemctl daemon-reload

echo "================================================="
echo "INFO: Script completed successfully."
echo "Final disk layout:"
lsblk
echo "Filesystem usage for the new mount:"
df -h "${MOUNT_POINT}"