#!/usr/bin/env bash
# Installe DevStack all-in-one sur Ubuntu 22.04 (VM fraîche, 8 Go RAM min, 4 vCPU, 60 Go disque).
# Usage : sudo ./devstack/install.sh
set -euo pipefail

DEVSTACK_BRANCH="${DEVSTACK_BRANCH:-stable/2025.2}"   # master ne teste plus Jammy
SWAP_GB="${SWAP_GB:-4}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

[[ $EUID -eq 0 ]] || { echo "Lance ce script avec sudo"; exit 1; }
. /etc/os-release
[[ "$VERSION_ID" == "22.04" ]] || echo "WARN: testé sur Ubuntu 22.04, détecté $VERSION_ID"

RAM_GB=$(awk '/MemTotal/ {printf "%d", $2/1024/1024}' /proc/meminfo)
echo "RAM détectée : ${RAM_GB} Go"
(( RAM_GB >= 7 )) || { echo "Moins de 8 Go : DevStack risque l'OOM. Abandon."; exit 1; }

# Swap (évite les OOM pendant stack.sh)
if ! swapon --show | grep -q /swapfile; then
  fallocate -l "${SWAP_GB}G" /swapfile && chmod 600 /swapfile
  mkswap /swapfile && swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

apt-get update && apt-get -y upgrade
apt-get install -y git curl

# Utilisateur stack
if ! id stack &>/dev/null; then
  useradd -s /bin/bash -d /opt/stack -m stack
  chmod +x /opt/stack
  echo "stack ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/stack
fi

HOST_IP=$(ip -4 route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++) if($i=="src") print $(i+1)}')
echo "HOST_IP=${HOST_IP}"

# Clone en tant que stack, puis local.conf généré en root (évite les droits sur /home/<user>, en 750 sur 22.04)
sudo -u stack -H bash -c "cd /opt/stack && [[ -d devstack ]] || git clone -b ${DEVSTACK_BRANCH} https://opendev.org/openstack/devstack"
sed "s/__HOST_IP__/${HOST_IP}/" "${SCRIPT_DIR}/local.conf" > /opt/stack/devstack/local.conf

# Pas de VT-x/AMD-V exposé (ex. Workstation sur Hyper-V) -> Nova en émulation QEMU
if ! grep -Eq '(vmx|svm)' /proc/cpuinfo && ! grep -q '^LIBVIRT_TYPE=' /opt/stack/devstack/local.conf; then
  echo "WARN: pas de virtualisation matérielle, LIBVIRT_TYPE=qemu"
  sed -i '/^\[\[local|localrc\]\]/a LIBVIRT_TYPE=qemu' /opt/stack/devstack/local.conf
fi
chown stack:stack /opt/stack/devstack/local.conf

sudo -u stack -H bash -c "cd /opt/stack/devstack && ./stack.sh"

echo
echo "OK. Pour utiliser la CLI / l'API :"
echo "  sudo -iu stack && source /opt/stack/devstack/openrc admin admin && openstack server list"
