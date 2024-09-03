#!/usr/bin/env bash
# standard bash error handling
set -eEuo pipefail

if [ "${DEBUG-}" = true ]; then
  set -x
fi

# Defaults
K8S_VERSION=${K8S_VERSION:-v1.27.0}
KUBECTL_VERSION=${KUBECTL_VERSION:-$K8S_VERSION}
ENGINE=${CLUSTER_ENGINE:-kind}
NODES=${NODES:-0}

# Define the directories used by the script
ROOT_DIR=$(cd "$(dirname "$0")/../"; pwd)
HACK_DIR="${ROOT_DIR}/hack"
E2E_DIR="${HACK_DIR}/e2e"
TEMP_DIR="$(mktemp -d)"
LOG_DIR=${LOG_DIR:-$ROOT_DIR/_logs/}
trap 'rm -fr ${TEMP_DIR}' EXIT

# Operating System and Architecture
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
  x86_64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac

# HELPER_IMGS
CNPG_IMG="ghcr.io/cloudnative-pg/cloudnative-pg:1.19.2"
INGRESS_NGINX_IMG="registry.k8s.io/ingress-nginx/controller:v1.5.1"
INGRESS_NGINX_HOOK_IMG="registry.k8s.io/ingress-nginx/kube-webhook-certgen:v20220916-gd32f8c343"
CURL_IMG="curlimages/curl:7.86.0"
NGINX_IMG="nginx:1.22.1"
HELPER_IMGS=("${CURL_IMG}" "${NGINX_IMG}")

# Colors (only if using a terminal)
bright=
reset=
if [ -t 1 ]; then
  bright=$(tput bold 2>/dev/null || true)
  reset=$(tput sgr0 2>/dev/null || true)
fi

##
## KIND SUPPORT
##

install_kind() {
  local bindir=$1
  local binary="${bindir}/kind"
  local version

  # Get the latest release of kind unless specified in the environment
  version=${KIND_VERSION:-$(
    curl -s -LH "Accept:application/json" https://github.com/kubernetes-sigs/kind/releases/latest |
      sed 's/.*"tag_name":"\([^"]\+\)".*/\1/'
  )}

  curl -s -L "https://kind.sigs.k8s.io/dl/${version}/kind-${OS}-${ARCH}" -o "${binary}"
  chmod +x "${binary}"
}

load_image_kind() {
  local cluster_name=$1
  local image=$2
  kind load -v 5 docker-image --name "${cluster_name}" "${image}"
}

create_cluster_kind() {
  local k8s_version=$1
  local cluster_name=$2

  # Create kind config
  config_file="${TEMP_DIR}/kind-config.yaml"
  cat >"${config_file}" <<-EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "0.0.0.0"
  kubeProxyMode: "ipvs"

# add to the apiServer certSANs the name of the docker (dind) service in order to be able to reach the cluster through it
kubeadmConfigPatchesJSON6902:
  - group: kubeadm.k8s.io
    version: v1beta2
    kind: ClusterConfiguration
    patch: |
      - op: add
        path: /apiServer/certSANs/-
        value: docker
nodes:
- role: control-plane
EOF

  if [ "$NODES" -gt 1 ]; then
    for ((i = 0; i < NODES; i++)); do
      echo '- role: worker' >>"${config_file}"
    done
  fi

  if [ -n "${DOCKER_REGISTRY_MIRROR:-}" ] || [ -n "${ENABLE_REGISTRY:-}" ]; then
    # Add containerdConfigPatches section
    cat >>"${config_file}" <<-EOF

containerdConfigPatches:
EOF

    if [ -n "${DOCKER_REGISTRY_MIRROR:-}" ]; then
      cat >>"${config_file}" <<-EOF
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
    endpoint = ["${DOCKER_REGISTRY_MIRROR}"]
EOF
    fi
  fi
  # Create the cluster
  kind create cluster --name "${cluster_name}" --image "kindest/node:${k8s_version}" --config "${config_file}"

  # Workaround for https://kind.sigs.k8s.io/docs/user/known-issues/#pod-errors-due-to-too-many-open-files
  for node in $(kind get nodes --name "${cluster_name}"); do
    docker exec "$node" sysctl fs.inotify.max_user_watches=524288 fs.inotify.max_user_instances=512
  done
}

export_logs_kind() {
  local cluster_name=$1
  kind export logs "${LOG_DIR}" --name "${cluster_name}"
}

destroy_kind() {
  local cluster_name=$1
  kind delete cluster --name "${cluster_name}" || true
  docker network rm "kind" &>/dev/null || true
}

##
## GENERIC ROUTINES
##

install_kubectl() {
  local bindir=$1

  local binary="${bindir}/kubectl"

  curl -sL "https://storage.googleapis.com/kubernetes-release/release/v${KUBECTL_VERSION#v}/bin/${OS}/${ARCH}/kubectl" -o "${binary}"
  chmod +x "${binary}"
}

deploy_pg() {
  local PG_IMAGE=ghcr.io/cloudnative-pg/cloudnative-pg:1.19.2

  docker pull "${CNPG_IMG}"
  load_image "${CLUSTER_NAME}" "${CNPG_IMG}"
  #load_image "${CLUSTER_NAME}" "${PG_IMAGE}"

  # Add cnpg for postgres in cluster
  kubectl apply -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.19/releases/cnpg-1.19.2.yaml

  # Run the tests and destroy the cluster
  # Do not fail out if the tests fail. We want the postgres anyway.
  ITER=0
  NODE=$(kubectl get nodes --no-headers | wc -l | tr -d " ")
  while true; do
    if [[ $ITER -ge 300 ]]; then
      echo "Time out waiting for cloud native Pg readiness"
      exit 1
    fi
    NUM_READY=$(kubectl get deployments -n cnpg-system cnpg-controller-manager -o jsonpath='{.status.readyReplicas}')
    if [[ "$NUM_READY" == "1" ]]; then
      echo "CloudNative PG is Ready"
      break
    fi
    sleep 1
    ((++ITER))
  done
}

deploy_ingress() {
  docker pull ${INGRESS_NGINX_IMG}
  docker pull ${INGRESS_NGINX_HOOK_IMG}
  load_image "${CLUSTER_NAME}" "${INGRESS_NGINX_IMG}"
  load_image "${CLUSTER_NAME}" "${INGRESS_NGINX_HOOK_IMG}"
  # Add ingress-nginx in cluster
  kubectl apply -k ${E2E_DIR}/nginx

  # Run the tests and destroy the cluster
  # Do not fail out if the tests fail. We want the postgres anyway.
  ITER=0
  NODE=$(kubectl get nodes --no-headers | wc -l | tr -d " ")
  while true; do
    if [[ $ITER -ge 300 ]]; then
      echo "Time out waiting for nginx ingress readiness"
      exit 1
    fi
    NUM_READY=$(kubectl get deployments -n ingress-nginx ingress-nginx-controller -o jsonpath='{.status.readyReplicas}')
    if [[ "$NUM_READY" == "1" ]]; then
      echo "nginx ingress is Ready"
      break
    fi
    sleep 1
    ((++ITER))
  done
}

deploy_fluentd() {
  local FLUENTD_IMAGE=fluent/fluentd-kubernetes-daemonset:v1.14.3-debian-forward-1.0

  docker pull "${FLUENTD_IMAGE}"
  load_image "${CLUSTER_NAME}" "${FLUENTD_IMAGE}"

  # Add fluentd service to export logs
  kubectl apply -f "${E2E_DIR}/local-fluentd.yaml"

  # Run the tests and destroy the cluster
  # Do not fail out if the tests fail. We want the logs anyway.
  ITER=0
  NODE=$(kubectl get nodes --no-headers | wc -l | tr -d " ")
  while true; do
    if [[ $ITER -ge 300 ]]; then
      echo "Time out waiting for FluentD readiness"
      exit 1
    fi
    NUM_READY=$(kubectl get ds fluentd -n kube-system -o jsonpath='{.status.numberReady}')
    if [[ "$NUM_READY" == "$NODE" ]]; then
      echo "FluentD is Ready"
      break
    fi
    sleep 1
    ((++ITER))
  done
}

load_helper_images() {
  echo "${bright}Loading helper images for tests on cluster ${CLUSTER_NAME}${reset}"

  # Here we pre-load all the images defined in the HELPER_IMGS variable
  # with the goal to speed up the runs.
  for IMG in "${HELPER_IMGS[@]}"; do
    docker pull "${IMG}"
    "load_image_${ENGINE}" "${CLUSTER_NAME}" "${IMG}"
  done

  echo "${bright}Done loading helper images on cluster ${CLUSTER_NAME}${reset}"
}

load_image() {
  local cluster_name=$1
  local image=$2
  "load_image_${ENGINE}" "${cluster_name}" "${image}"
}

deploy_operator() {
  kubectl delete ns bim-system 2> /dev/null || :

  make -C "${ROOT_DIR}" deploy "CONTROLLER_IMG=${CONTROLLER_IMG}"
}

usage() {
  cat >&2 <<EOF
Usage: $0 [-e {kind|k3d}] [-k <version>] [-r] <command>

Commands:
    prepare <dest_dir>    Downloads the prerequisite into <dest_dir>
    create                Create the test cluster
    load                  Build and load the operator image in the cluster
    deploy                Deploy the operator manifests in the cluster
    print-image           Print the CONTROLLER_IMG name to be used inside
                          the cluster
    export-logs           Export the logs from the cluster inside the directory
                          ${LOG_DIR}
    destroy               Destroy the cluster

Options:
    -k|--k8s-version
        <K8S_VERSION>     Use the specified kubernetes full version number
                          (e.g., v1.25.0). Env: K8S_VERSION

    -n|--nodes
        <NODES>           Create a cluster with the required number of nodes.
                          Used only during "create" command. Default: 3
                          Env: NODES

To use long options you need to have GNU enhanced getopt available, otherwise
you can only use the short version of the options.
EOF
  exit 1
}

##
## COMMANDS
##

prepare() {
  local bindir=$1
  echo "${bright}Installing cluster prerequisites in ${bindir}${reset}"
  install_kubectl "${bindir}"
  "install_${ENGINE}" "${bindir}"
  echo "${bright}Done installing cluster prerequisites in ${bindir}${reset}"
}

create() {
  echo "${bright}Creating ${ENGINE} cluster ${CLUSTER_NAME} with version ${K8S_VERSION}${reset}"

  "create_cluster_${ENGINE}" "${K8S_VERSION}" "${CLUSTER_NAME}"

  # Support for docker:dind service
  if [ "${DOCKER_HOST:-}" == "tcp://docker:2376" ]; then
    sed -i -E -e 's/0\.0\.0\.0/docker/g' "${HOME}/.kube/config"
  fi

  deploy_fluentd
  deploy_pg

  if [ "$INGRESS_CLASS" == "nginx" ]; then
    deploy_ingress
  fi

  echo "${bright}Done creating ${ENGINE} cluster ${CLUSTER_NAME} with version ${K8S_VERSION}${reset}"
}

load() {
  echo "${bright}Building operator from current worktree${reset}"

  CONTROLLER_IMG="$(print_image)"
  make -C "${ROOT_DIR}" CONTROLLER_IMG="${CONTROLLER_IMG}" ARCH="${ARCH}" docker-build

  echo "${bright}Loading new operator image on cluster ${CLUSTER_NAME}${reset}"

  load_image "${CLUSTER_NAME}" "${CONTROLLER_IMG}"

  echo "${bright}Done loading new operator image on cluster ${CLUSTER_NAME}${reset}"
}

deploy() {
  CONTROLLER_IMG="$(print_image)"

  echo "${bright}Deploying manifests from current worktree on cluster ${CLUSTER_NAME}${reset}"

  deploy_operator

  echo "${bright}Done deploying manifests from current worktree on cluster ${CLUSTER_NAME}${reset}"
}

print_image() {
  echo "bibook.com/bibook-ingress-manager:latest"
}

export_logs() {
  echo "${bright}Exporting logs from cluster ${CLUSTER_NAME} to ${LOG_DIR}${reset}"

  "export_logs_${ENGINE}" "${CLUSTER_NAME}"

  echo "${bright}Done exporting logs from cluster ${CLUSTER_NAME} to ${LOG_DIR}${reset}"
}

destroy() {
  echo "${bright}Destroying ${ENGINE} cluster ${CLUSTER_NAME}${reset}"

  "destroy_${ENGINE}" "${CLUSTER_NAME}"

  echo "${bright}Done destroying ${ENGINE} cluster ${CLUSTER_NAME}${reset}"
}

##
## MAIN
##

main() {
  if ! getopt -T > /dev/null; then
    # GNU enhanced getopt is available
    parsed_opts=$(getopt -o e:k:n:r -l "engine:,k8s-version:,nodes:,registry" -- "$@") || usage
  else
    # Original getopt is available
    parsed_opts=$(getopt e:k:n:r "$@") || usage
  fi
  eval "set -- $parsed_opts"
  for o; do
    case "${o}" in
    -k | --k9s-version)
      shift
      K8S_VERSION="v${1#v}"
      shift
      if ! [[ $K8S_VERSION =~ ^v1\.[0-9]+\.[0-9]+$ ]]; then
        echo "ERROR: $K8S_VERSION is not a valid k8s version!" >&2
        echo >&2
        usage
      fi
      ;;
    -n | --nodes)
      shift
      NODES="${1}"
      shift
      if ! [[ $NODES =~ ^[1-9][0-9]*$ ]]; then
        echo "ERROR: $NODES is not a positive integer!" >&2
        echo >&2
        usage
      fi
      ;;
    --)
      shift
      break
      ;;
    esac
  done

  # Check if command is missing
  if [ "$#" -eq 0 ]; then
    echo "ERROR: you must specify a command" >&2
    echo >&2
    usage
  fi

  # Only here the K8S_VERSION veriable contains its final value
  # so we can set the default cluster name
  CLUSTER_NAME=${CLUSTER_NAME:-bim-operator-e2e-${K8S_VERSION//./-}}

  while [ "$#" -gt 0 ]; do
    command=$1
    shift

    # Invoke the command
    case "$command" in
    prepare)
      if [ "$#" -eq 0 ]; then
        echo "ERROR: prepare requires a destination directory" >&2
        echo >&2
        usage
      fi
      dest_dir=$1
      shift
      prepare "${dest_dir}"
      ;;

    create | load | load-helper-images | deploy | print-image | export-logs | destroy)
      "${command//-/_}"
      ;;
    *)
      echo "ERROR: unknown command ${command}" >&2
      echo >&2
      usage
      ;;
    esac
  done
}

main "$@"