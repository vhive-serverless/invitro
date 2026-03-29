#!/bin/bash
# Exit immediately if a command exits with a non-zero status.
set -euo pipefail

# --- Configuration ---
MOUNT_POINT="/mnt/resources/minio"
TMPFS_MOUNT="/mnt/tmpfs"
RAMDISK_IMAGE_NAME="ramdisk.img"
RAMDISK_SIZE_GB="96"
FILESYSTEM="xfs"
RAMDISK_IMAGE_PATH="${TMPFS_MOUNT}/${RAMDISK_IMAGE_NAME}"

echo "================================================="
echo "INFO: Preparing RAM disk for MinIO"
echo "INFO: Mount point      : ${MOUNT_POINT}"
echo "INFO: Tmpfs mount      : ${TMPFS_MOUNT}"
echo "INFO: Ramdisk image    : ${RAMDISK_IMAGE_PATH}"
echo "INFO: Ramdisk size (GB): ${RAMDISK_SIZE_GB}"
echo "================================================="

# --- Cleanup existing mounts/devices ---

echo "INFO: Cleaning up any existing MinIO RAM-disk mounts..."
sudo umount -f -l "${MOUNT_POINT}" 2>/dev/null || true

while read -r loopdev _; do
    clean_loopdev="${loopdev%:}"
    if [[ -n "${clean_loopdev}" ]]; then
        echo "INFO: Detaching loop device ${clean_loopdev}"
        sudo losetup -d "${clean_loopdev}" || true
    fi
done < <(sudo losetup -j "${RAMDISK_IMAGE_PATH}" || true)

sudo umount -f -l "${TMPFS_MOUNT}" 2>/dev/null || true
sudo rm -rf "${MOUNT_POINT}" "${TMPFS_MOUNT}"

# --- Create RAM disk ---

echo "INFO: Creating tmpfs mount at ${TMPFS_MOUNT}..."
sudo mkdir -p "${TMPFS_MOUNT}"
sudo mount -t tmpfs -o "size=${RAMDISK_SIZE_GB}G" tmpfs "${TMPFS_MOUNT}"

echo "INFO: Creating ${RAMDISK_SIZE_GB}G RAM-disk image..."
RAMDISK_SIZE_MB=$((RAMDISK_SIZE_GB * 1024))
sudo dd if=/dev/zero of="${RAMDISK_IMAGE_PATH}" bs=1M count="${RAMDISK_SIZE_MB}" status=progress

echo "INFO: Attaching loop device..."
LOOPDEV=$(sudo losetup --show -f "${RAMDISK_IMAGE_PATH}")
echo "INFO: Loop device is ${LOOPDEV}"

echo "INFO: Formatting ${LOOPDEV} with ${FILESYSTEM}..."
sudo mkfs."${FILESYSTEM}" -f "${LOOPDEV}"

echo "INFO: Creating mount point ${MOUNT_POINT}..."
sudo mkdir -p "${MOUNT_POINT}"
sudo chmod 777 "${MOUNT_POINT}"

echo "INFO: Mounting ${LOOPDEV} to ${MOUNT_POINT}..."
sudo mount -t "${FILESYSTEM}" "${LOOPDEV}" "${MOUNT_POINT}"

echo "================================================="
echo "INFO: Script completed successfully."
echo "Loop device mapping:"
sudo losetup -a | grep "${RAMDISK_IMAGE_PATH}" || true
echo "Filesystem usage:"
df -h "${MOUNT_POINT}"