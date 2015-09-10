#!/bin/bash
#
# This scripts starts the OpenShift server with a default configuration.
# The OpenShift Docker registry and router are installed.
# It will start all 'default_*_test.go' test cases.

set -o nounset
set -o pipefail

OS_ROOT=$(dirname "${BASH_SOURCE}")/../../..
cd "${OS_ROOT}"

source ${OS_ROOT}/hack/util.sh
source ${OS_ROOT}/hack/common.sh


set -e
ensure_ginkgo_or_die
set +e

os::build::extended

ensure_iptables_or_die

function cleanup()
{
	out=$?
	cleanup_openshift
	echo "[INFO] Exiting"
	exit $out
}

echo "[INFO] Starting 'builds' extended tests"

trap "exit" INT TERM
trap "cleanup" EXIT

export TMPDIR="${TMPDIR:-"/tmp"}"
export BASETMPDIR="${TMPDIR}/openshift-extended-tests/builds"
setup_env_vars
reset_tmp_dir 
configure_os_server
start_os_server

install_registry
wait_for_registry

echo "[INFO] Creating image streams"
oc create -n openshift -f examples/image-streams/image-streams-centos7.json --config="${ADMIN_KUBECONFIG}"

# Run the tests
pushd ${OS_ROOT}/test/extended >/dev/null
export KUBECONFIG="${ADMIN_KUBECONFIG}"
export EXTENDED_TEST_PATH="${OS_ROOT}/test/extended"

# run the parallel half of the bucket
TMPDIR=${BASETMPDIR} ginkgo -progress -stream -v -focus="builds: parallel:" -p ${OS_OUTPUT_BINPATH}/extended.test

# run the serial half of the buckets
TMPDIR=${BASETMPDIR} ginkgo -progress -stream -v -focus="builds: serial:" ${OS_OUTPUT_BINPATH}/extended.test

popd >/dev/null


