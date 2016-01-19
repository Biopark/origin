#debuginfo not supported with Go
%global debug_package %{nil}
# modifying the Go binaries breaks the DWARF debugging
%global __os_install_post %{_rpmconfigdir}/brp-compress

%global gopath      %{_datadir}/gocode
%global import_path github.com/openshift/origin
%global sdn_import_path github.com/openshift/openshift-sdn
# The following should only be used for cleanup of sdn-ovs upgrades
%global kube_plugin_path /usr/libexec/kubernetes/kubelet-plugins/net/exec/redhat~openshift-ovs-subnet

# docker_version is the version of docker requires by packages
%global docker_version 1.8.2
# tuned_version is the version of tuned requires by packages
%global tuned_version  2.3
# openvswitch_version is the version of openvswitch requires by packages
%global openvswitch_version 2.3.1
# this is the version we obsolete up to. The packaging changed for Origin
# 1.0.6 and OSE 3.1 such that 'openshift' package names were no longer used.
%global package_refector_version 3.0.2.900
# %commit and %ldflags are intended to be set by tito custom builders provided
# in the .tito/lib directory. The values in this spec file will not be kept up to date.
%{!?commit:
%global commit 45bc219e1f0a1642fa15071d8610bb8b41d93c24
}
%global shortcommit %(c=%{commit}; echo ${c:0:7})
# ldflags from hack/common.sh os::build:ldflags
%{!?ldflags:
%global ldflags -X github.com/openshift/origin/pkg/version.majorFromGit 3 -X github.com/openshift/origin/pkg/version.minorFromGit 1+ -X github.com/openshift/origin/pkg/version.versionFromGit v3.1.1.4-8-g45bc219 -X github.com/openshift/origin/pkg/version.commitFromGit 45bc219 -X k8s.io/kubernetes/pkg/version.gitCommit 4c8e6f4 -X k8s.io/kubernetes/pkg/version.gitVersion v1.1.0-origin-1107-g4c8e6f4
}

%if 0%{?fedora} || 0%{?epel}
%global make_redistributable 0
%else
%global make_redistributable 1
%endif

%if "%{dist}" == ".el7aos"
%global package_name atomic-openshift
%global product_name Atomic OpenShift
%else
%global package_name origin
%global product_name Origin
%endif

Name:           atomic-openshift
# Version is not kept up to date and is intended to be set by tito custom
# builders provided in the .tito/lib directory of this project
Version:        3.1.1.5
Release:        1%{?dist}
Summary:        Open Source Container Management by Red Hat
License:        ASL 2.0
URL:            https://%{import_path}
ExclusiveArch:  x86_64
Source0:        https://%{import_path}/archive/%{commit}/%{name}-%{version}.tar.gz
BuildRequires:  systemd
BuildRequires:  golang >= 1.4
Requires:       %{name}-clients = %{version}-%{release}
Requires:       iptables
Obsoletes:      openshift < %{package_refector_version}

#
# The following Bundled Provides entries are populated automatically by the
# OpenShift Origin tito custom builder found here:
#   https://github.com/openshift/origin/blob/master/.tito/lib/origin/builder/
#
# These are defined as per:
# https://fedoraproject.org/wiki/Packaging:Guidelines#Bundling_and_Duplication_of_system_libraries
#
### AUTO-BUNDLED-GEN-ENTRY-POINT

%description
Origin is a distribution of Kubernetes optimized for enterprise application
development and deployment, used by OpenShift 3 and Atomic Enterprise. Origin
adds developer and operational centric tools on top of Kubernetes to enable
rapid application development, easy deployment and scaling, and long-term
lifecycle maintenance for small and large teams and applications.

%package master
Summary:        %{product_name} Master
Requires:       %{name} = %{version}-%{release}
Requires(post):   systemd
Requires(preun):  systemd
Requires(postun): systemd
Obsoletes:      openshift-master < %{package_refector_version}

%description master
%{summary}

%package node
Summary:        %{product_name} Node
Requires:       %{name} = %{version}-%{release}
Requires:       docker-io >= %{docker_version}
Requires:       tuned-profiles-%{name}-node = %{version}-%{release}
Requires:       util-linux
Requires:       socat
Requires:       nfs-utils
Requires:       ethtool
Requires(post):   systemd
Requires(preun):  systemd
Requires(postun): systemd
Obsoletes:      openshift-node < %{package_refector_version}

%description node
%{summary}

%package -n tuned-profiles-%{name}-node
Summary:        Tuned profiles for %{product_name} Node hosts
Requires:       tuned >= %{tuned_version}
Obsoletes:      tuned-profiles-openshift-node < %{package_refector_version}

%description -n tuned-profiles-%{name}-node
%{summary}

%package clients
Summary:        %{product_name} Client binaries for Linux
Obsoletes:      openshift-clients < %{package_refector_version}
Requires:       git

%description clients
%{summary}

%if 0%{?make_redistributable}
%package clients-redistributable
Summary:        %{product_name} Client binaries for Linux, Mac OSX, and Windows
BuildRequires:  golang-pkg-darwin-amd64
BuildRequires:  golang-pkg-windows-386
Obsoletes:      openshift-clients-redistributable < %{package_refector_version}

%description clients-redistributable
%{summary}
%endif

%package dockerregistry
Summary:        Docker Registry v2 for %{product_name}
Requires:       %{name} = %{version}-%{release}

%description dockerregistry
%{summary}

%package pod
Summary:        %{product_name} Pod
Requires:       %{name} = %{version}-%{release}

%description pod
%{summary}

%package recycle
Summary:        %{product_name} Recycler
Requires:       %{name} = %{version}-%{release}

%description recycle
%{summary}

%package sdn-ovs
Summary:          %{product_name} SDN Plugin for Open vSwitch
Requires:         openvswitch >= %{openvswitch_version}
Requires:         %{name}-node = %{version}-%{release}
Requires:         bridge-utils
Requires:         ethtool
Requires:         procps-ng
Requires:         iproute
Obsoletes:        openshift-sdn-ovs < %{package_refector_version}

%description sdn-ovs
%{summary}

%prep
%setup -q

%build

# Don't judge me for this ... it's so bad.
mkdir _build

# Horrid hack because golang loves to just bundle everything
pushd _build
    mkdir -p src/github.com/openshift
    ln -s $(dirs +1 -l) src/%{import_path}
popd


# Gaming the GOPATH to include the third party bundled libs at build
# time.
mkdir _thirdpartyhacks
pushd _thirdpartyhacks
    ln -s \
        $(dirs +1 -l)/Godeps/_workspace/src/ \
            src
popd
export GOPATH=$(pwd)/_build:$(pwd)/_thirdpartyhacks:%{buildroot}%{gopath}:%{gopath}
# Build all linux components we care about
for cmd in oc openshift dockerregistry recycle
do
        go install -ldflags "%{ldflags}" %{import_path}/cmd/${cmd}
done

%if 0%{?make_redistributable}
# Build clients for other platforms
GOOS=windows GOARCH=386 go install -ldflags "%{ldflags}" %{import_path}/cmd/oc
GOOS=darwin GOARCH=amd64 go install -ldflags "%{ldflags}" %{import_path}/cmd/oc
%endif

#Build our pod
pushd images/pod/
    go build -ldflags "%{ldflags}" pod.go
popd

%install

install -d %{buildroot}%{_bindir}

# Install linux components
for bin in oc openshift dockerregistry recycle
do
  echo "+++ INSTALLING ${bin}"
  install -p -m 755 _build/bin/${bin} %{buildroot}%{_bindir}/${bin}
done

%if 0%{?make_redistributable}
# Install client executable for windows and mac
install -d %{buildroot}%{_datadir}/%{name}/{linux,macosx,windows}
install -p -m 755 _build/bin/oc %{buildroot}%{_datadir}/%{name}/linux/oc
install -p -m 755 _build/bin/darwin_amd64/oc %{buildroot}/%{_datadir}/%{name}/macosx/oc
install -p -m 755 _build/bin/windows_386/oc.exe %{buildroot}/%{_datadir}/%{name}/windows/oc.exe
%endif

#Install pod
install -p -m 755 images/pod/pod %{buildroot}%{_bindir}/

install -d -m 0755 %{buildroot}%{_unitdir}

mkdir -p %{buildroot}%{_sysconfdir}/sysconfig

for cmd in \
    openshift-router \
    openshift-deploy \
    openshift-sti-build \
    openshift-docker-build \
    origin \
    atomic-enterprise \
    oadm \
    kubernetes \
    kubelet \
    kube-proxy \
    kube-apiserver \
    kube-controller-manager \
    kube-scheduler
do
    ln -s %{_bindir}/openshift %{buildroot}%{_bindir}/$cmd
done

ln -s oc %{buildroot}%{_bindir}/kubectl

install -d -m 0755 %{buildroot}%{_sysconfdir}/origin/{master,node}

# different service for origin vs aos
install -m 0644 contrib/systemd/%{name}-master.service %{buildroot}%{_unitdir}/%{name}-master.service
install -m 0644 contrib/systemd/%{name}-node.service %{buildroot}%{_unitdir}/%{name}-node.service
# same sysconfig files for origin vs aos
install -m 0644 contrib/systemd/origin-master.sysconfig %{buildroot}%{_sysconfdir}/sysconfig/%{name}-master
install -m 0644 contrib/systemd/origin-node.sysconfig %{buildroot}%{_sysconfdir}/sysconfig/%{name}-node
install -d -m 0755 %{buildroot}%{_prefix}/lib/tuned/%{name}-node-{guest,host}
install -m 0644 contrib/tuned/origin-node-guest/tuned.conf %{buildroot}%{_prefix}/lib/tuned/%{name}-node-guest/tuned.conf
install -m 0644 contrib/tuned/origin-node-host/tuned.conf %{buildroot}%{_prefix}/lib/tuned/%{name}-node-host/tuned.conf
install -d -m 0755 %{buildroot}%{_mandir}/man7

# Patch the manpage for tuned profiles on aos
%if "%{dist}" == ".el7aos"
%{__sed} -e 's|origin-node|atomic-openshift-node|g' \
 -e 's|ORIGIN_NODE|ATOMIC_OPENSHIFT_NODE|' \
 contrib/tuned/man/tuned-profiles-origin-node.7 > %{buildroot}%{_mandir}/man7/tuned-profiles-%{name}-node.7
%else
install -m 0644 contrib/tuned/man/tuned-profiles-origin-node.7 %{buildroot}%{_mandir}/man7/tuned-profiles-%{name}-node.7
%endif

mkdir -p %{buildroot}%{_sharedstatedir}/origin


# Install sdn scripts
install -d -m 0755 %{buildroot}%{_unitdir}/docker.service.d
install -p -m 0644 contrib/systemd/docker-sdn-ovs.conf %{buildroot}%{_unitdir}/docker.service.d/
pushd _thirdpartyhacks/src/%{sdn_import_path}/plugins/osdn/ovs/bin
   install -p -m 755 openshift-sdn-ovs %{buildroot}%{_bindir}/openshift-sdn-ovs
   install -p -m 755 openshift-sdn-docker-setup.sh %{buildroot}%{_bindir}/openshift-sdn-docker-setup.sh
popd
install -d -m 0755 %{buildroot}%{_unitdir}/%{name}-node.service.d
install -p -m 0644 contrib/systemd/openshift-sdn-ovs.conf %{buildroot}%{_unitdir}/%{name}-node.service.d/openshift-sdn-ovs.conf

# Install bash completions
install -d -m 755 %{buildroot}%{_sysconfdir}/bash_completion.d/
install -p -m 644 contrib/completions/bash/* %{buildroot}%{_sysconfdir}/bash_completion.d/
# Generate atomic-enterprise bash completions
%{__sed} -e "s|openshift|atomic-enterprise|g" contrib/completions/bash/openshift > %{buildroot}%{_sysconfdir}/bash_completion.d/atomic-enterprise

%files
%doc README.md
%license LICENSE
%{_bindir}/openshift
%{_bindir}/openshift-router
%{_bindir}/openshift-deploy
%{_bindir}/openshift-sti-build
%{_bindir}/openshift-docker-build
%{_bindir}/origin
%{_bindir}/atomic-enterprise
%{_bindir}/oadm
%{_bindir}/kubernetes
%{_bindir}/kubelet
%{_bindir}/kube-proxy
%{_bindir}/kube-apiserver
%{_bindir}/kube-controller-manager
%{_bindir}/kube-scheduler
%{_sharedstatedir}/origin
%{_sysconfdir}/bash_completion.d/atomic-enterprise
%{_sysconfdir}/bash_completion.d/oadm
%{_sysconfdir}/bash_completion.d/openshift
%dir %config(noreplace) %{_sysconfdir}/origin
%ghost %dir %config(noreplace) %{_sysconfdir}/origin
%ghost %config(noreplace) %{_sysconfdir}/origin/.config_managed

%pre
# If /etc/openshift exists and /etc/origin doesn't, symlink it to /etc/origin
if [ -d "%{_sysconfdir}/openshift" ]; then
  if ! [ -d "%{_sysconfdir}/origin"  ]; then
    ln -s %{_sysconfdir}/openshift %{_sysconfdir}/origin
  fi
fi
if [ -d "%{_sharedstatedir}/openshift" ]; then
  if ! [ -d "%{_sharedstatedir}/origin"  ]; then
    ln -s %{_sharedstatedir}/openshift %{_sharedstatedir}/origin
  fi
fi


%files master
%{_unitdir}/%{name}-master.service
%config(noreplace) %{_sysconfdir}/sysconfig/%{name}-master
%config(noreplace) %{_sysconfdir}/origin/master
%ghost %config(noreplace) %{_sysconfdir}/origin/admin.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/admin.key
%ghost %config(noreplace) %{_sysconfdir}/origin/admin.kubeconfig
%ghost %config(noreplace) %{_sysconfdir}/origin/ca.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/ca.key
%ghost %config(noreplace) %{_sysconfdir}/origin/ca.serial.txt
%ghost %config(noreplace) %{_sysconfdir}/origin/etcd.server.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/etcd.server.key
%ghost %config(noreplace) %{_sysconfdir}/origin/master-config.yaml
%ghost %config(noreplace) %{_sysconfdir}/origin/master.etcd-client.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/master.etcd-client.key
%ghost %config(noreplace) %{_sysconfdir}/origin/master.kubelet-client.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/master.kubelet-client.key
%ghost %config(noreplace) %{_sysconfdir}/origin/master.server.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/master.server.key
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-master.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-master.key
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-master.kubeconfig
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-registry.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-registry.key
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-registry.kubeconfig
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-router.crt
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-router.key
%ghost %config(noreplace) %{_sysconfdir}/origin/openshift-router.kubeconfig
%ghost %config(noreplace) %{_sysconfdir}/origin/policy.json
%ghost %config(noreplace) %{_sysconfdir}/origin/serviceaccounts.private.key
%ghost %config(noreplace) %{_sysconfdir}/origin/serviceaccounts.public.key
%ghost %config(noreplace) %{_sysconfdir}/origin/.config_managed

%post master
%systemd_post %{name}-master.service
# Create master config and certs if both do not exist
if [[ ! -e %{_sysconfdir}/origin/master/master-config.yaml &&
     ! -e %{_sysconfdir}/origin/master/ca.crt ]]; then
  %{_bindir}/openshift start master --write-config=%{_sysconfdir}/origin/master
  # Create node configs if they do not already exist
  if ! find %{_sysconfdir}/origin/ -type f -name "node-config.yaml" | grep -E "node-config.yaml"; then
    %{_bindir}/oadm create-node-config --node-dir=%{_sysconfdir}/origin/node/ --node=localhost --hostnames=localhost,127.0.0.1 --node-client-certificate-authority=%{_sysconfdir}/origin/master/ca.crt --signer-cert=%{_sysconfdir}/origin/master/ca.crt --signer-key=%{_sysconfdir}/origin/master/ca.key --signer-serial=%{_sysconfdir}/origin/master/ca.serial.txt --certificate-authority=%{_sysconfdir}/origin/master/ca.crt
  fi
  # Generate a marker file that indicates config and certs were RPM generated
  echo "# Config generated by RPM at "`date -u` > %{_sysconfdir}/origin/.config_managed
fi


%preun master
%systemd_preun %{name}-master.service

%postun master
%systemd_postun

%files node
%{_unitdir}/%{name}-node.service
%config(noreplace) %{_sysconfdir}/sysconfig/%{name}-node
%config(noreplace) %{_sysconfdir}/origin/node
%ghost %config(noreplace) %{_sysconfdir}/origin/.config_managed

%post node
%systemd_post %{name}-node.service

%preun node
%systemd_preun %{name}-node.service

%postun node
%systemd_postun

%files sdn-ovs
%dir %{_unitdir}/docker.service.d/
%dir %{_unitdir}/%{name}-node.service.d/
%{_bindir}/openshift-sdn-ovs
%{_bindir}/openshift-sdn-docker-setup.sh
%{_unitdir}/%{name}-node.service.d/openshift-sdn-ovs.conf
%{_unitdir}/docker.service.d/docker-sdn-ovs.conf

%posttrans sdn-ovs
# This path was installed by older packages but the directory wasn't owned by
# RPM so we need to clean it up otherwise kubelet throws an error trying to
# load the directory as a plugin
if [ -d %{kube_plugin_path} ]; then
  rmdir %{kube_plugin_path}
fi

%files -n tuned-profiles-%{name}-node
%license LICENSE
%{_prefix}/lib/tuned/%{name}-node-host
%{_prefix}/lib/tuned/%{name}-node-guest
%{_mandir}/man7/tuned-profiles-%{name}-node.7*

%post -n tuned-profiles-%{name}-node
recommended=`/usr/sbin/tuned-adm recommend`
if [[ "${recommended}" =~ guest ]] ; then
  /usr/sbin/tuned-adm profile %{name}-node-guest > /dev/null 2>&1
else
  /usr/sbin/tuned-adm profile %{name}-node-host > /dev/null 2>&1
fi

%preun -n tuned-profiles-%{name}-node
# reset the tuned profile to the recommended profile
# $1 = 0 when we're being removed > 0 during upgrades
if [ "$1" = 0 ]; then
  recommended=`/usr/sbin/tuned-adm recommend`
  /usr/sbin/tuned-adm profile $recommended > /dev/null 2>&1
fi

%files clients
%license LICENSE
%{_bindir}/oc
%{_bindir}/kubectl
%{_sysconfdir}/bash_completion.d/oc

%if 0%{?make_redistributable}
%files clients-redistributable
%dir %{_datadir}/%{name}/linux/
%dir %{_datadir}/%{name}/macosx/
%dir %{_datadir}/%{name}/windows/
%{_datadir}/%{name}/linux/oc
%{_datadir}/%{name}/macosx/oc
%{_datadir}/%{name}/windows/oc.exe
%endif

%files dockerregistry
%{_bindir}/dockerregistry

%files pod
%{_bindir}/pod

%files recycle
%{_bindir}/recycle


%changelog
* Mon Jan 18 2016 Troy Dawson <tdawson@redhat.com> 3.1.1.5
- More fixes and tweeks (tdawson@redhat.com)
- Lock ace-builds version to 1.2.2 (spadgett@redhat.com)
- Do not check builds/details in build by strategy admission control
  (cewong@redhat.com)

* Sat Jan 16 2016 Troy Dawson <tdawson@redhat.com> 3.1.1.4
- Fix up net.bridge.bridge-nf-call-iptables after kubernetes breaks it
  (danw@redhat.com)
- admission tests and swagger (pweil@redhat.com)
- UPSTREAM: <carry>: capability defaulting (pweil@redhat.com)
- Fix logic to add Dockerfile to BuildConfig (rhcarvalho@gmail.com)
- Add TIMES=N to rerun integration tests for flakes (ccoleman@redhat.com)
- Fix route serialization flake (mkargaki@redhat.com)
- STI -> S2I (dmcphers@redhat.com)
- Enable PostgreSQL replication tests for RHEL images (nagy.martin@gmail.com)

* Thu Jan 14 2016 Troy Dawson <tdawson@redhat.com> 3.1.1.3
- Bug 1298457 - The link to pv doc is wrong (bleanhar@redhat.com)
- Fix typo in build generator (mfojtik@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  eda3808d8fe615229f168661fea021a074a34750 (dcbw@redhat.com)
- Auto generated bash completions for node-ip kubelet config option
  (rpenta@redhat.com)
- Use KubeletServer.NodeIP instead of KubeletServer.HostnameOverride to set
  node IP (rpenta@redhat.com)
- UPSTREAM: <carry>: Tolerate node ExternalID changes with no cloud provider
  (sross@redhat.com)
- Update certs for router tests (jliggitt@redhat.com)
- Retry adding roles to service accounts in conflict cases
  (jliggitt@redhat.com)
- Include update operation in build admission controller (cewong@redhat.com)
- UPSTREAM: 18541: Allow node IP to be passed as optional config for kubelet
  (rpenta@redhat.com)
- Fix test fixture fields (mkargaki@redhat.com)
- Make `oc cancel-build` to be suggested for `oc stop-build`.
  (vsemushi@redhat.com)
- The default codec should be v1.Codec, not v1beta3 (ccoleman@redhat.com)

* Wed Jan 13 2016 Troy Dawson <tdawson@redhat.com> 3.1.1.2
- Wait for access tokens to be available in clustered etcd
  (jliggitt@redhat.com)
- Add a new image to be used for testing (ccoleman@redhat.com)
- Bug 1263609 - fix oc rsh usage (ffranz@redhat.com)
- Fix HPA default policy (jliggitt@redhat.com)
- oc rsync: expose additional rsync flags (cewong@redhat.com)
- Bug 1248463 - fixes exec help (ffranz@redhat.com)
- Bug 1273708 - mark --all in oc export deprecated in help (ffranz@redhat.com)
- Fix tests for route validation changes (mkargaki@redhat.com)
- Make new-build output BC with multiple sources (rhcarvalho@gmail.com)
- Remove code duplication (rhcarvalho@gmail.com)
- Require tls termination in route tls configuration (mkargaki@redhat.com)
- Fix nw extended test support for skipping build (marun@redhat.com)
- Fix broken switch in usageWithUnits filter (spadgett@redhat.com)
- UPSTREAM: 19481: make patch call update admission chain after applying the
  patch (deads@redhat.com)
- diagnostics: logs and units for origin (lmeyer@redhat.com)
- Update java console to 1.0.39 (slewis@fusesource.com)
- extended tests for jenkins openshift V3 plugin (gmontero@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  da8ad5dc5c94012eb222221d909b2b6fa678500f (dcbw@redhat.com)
- Update for openshift-sdn script installation changes (danw@redhat.com)
- use direct mount for etcd data (deads@redhat.com)
- mark image input source as experimental (bparees@redhat.com)
- made large file behavior smarter (skuznets@redhat.com)
- Revert "Allow parallel image stream importing" (jordan@liggitt.net)
- version now updates FROM (tdawson@redhat.com)
- Fix deployment CLI ops link and make all doc links https
  (jforrest@redhat.com)
- Fix detection of Python projects (rhcarvalho@gmail.com)
- add probe for mongodb template (haowang@redhat.com)
- deal with RawPath field added to url.URL in go1.5 (gmontero@redhat.com)
- Include kube e2e service tests in networking suite (marun@redhat.com)
- Fix replication controller usage in kube e2e tests (marun@redhat.com)
- Enable openshift-sdn sdn node by default (marun@redhat.com)
- Fix dind compatibility with centos/rhel (marun@redhat.com)
- Fix dind compatibility with centos/rhel (marun@redhat.com)
- added junitreport tool (skuznets@redhat.com)
- diagnostics: list diagnostic names in long desc (lmeyer@redhat.com)
- Increase web console e2e login timeout (spadgett@redhat.com)
- Persistent volume claims on the web console (ffranz@redhat.com)
- update extended test to point to correct version tool (skuznets@redhat.com)
- add warning about root user in images (bparees@redhat.com)
- allow parallel image streams (deads@redhat.com)
- Suppress conflict error logging when adding SA role bindings
  (jliggitt@redhat.com)
- BuildConfig envVars in wrong structure in sti build template
  (jhadvig@redhat.com)
- Remove unnecessary type conversions (rhcarvalho@gmail.com)

* Thu Jan 07 2016 Troy Dawson <tdawson@redhat.com> 3.1.1.1
- add option --insecure for oc import-image
  (haoran@dhcp-129-204.nay.redhat.com)
- new-app: search local docker daemon if registry search fails
  (cewong@redhat.com)
- Fix breadcrumb on next steps page (spadgett@redhat.com)
- Enable DWARF debuginfo (tdawson@redhat.com)
- update scripts to respect TMPDIR (deads@redhat.com)
- handle missing dockerfile with docker strategy (bparees@redhat.com)
- Bump kubernetes-topology-graph to 0.0.21 (spadgett@redhat.com)
- Print more enlightening string if test fails (rhcarvalho@gmail.com)
- Add to project catalog legend and accessibility fixes (spadgett@redhat.com)
- tolerate spurious failure during test setup (deads@redhat.com)
- fixed readiness endpoint route listing (skuznets@redhat.com)
- moved tools from cmd/ to tools/ (skuznets@redhat.com)
- Fix scale up button tooltip (spadgett@redhat.com)
- fix deploy test to use actual master (deads@redhat.com)
- Use angular-bootstrap dropdown for user menu (spadgett@redhat.com)
- added bash autocompletion for ldap sync config (skuznets@redhat.com)
- Don't allow clock icon to wrap in image tag table (spadgett@redhat.com)
- diagnostics: improve wording of notes (lmeyer@redhat.com)
- diagnostics: improve master/node config warnings (lmeyer@redhat.com)
- Show scalable deployments on web console overview even if not latest
  (spadgett@redhat.com)
- Use angular-bootstrap uib-prefixed components (spadgett@redhat.com)
- write output to file in e2e core (skuznets@redhat.com)
- Make web console alerts dismissable (spadgett@redhat.com)
- Reenabled original registry's /healthz route (miminar@redhat.com)
- fixed TestEditor output (skuznets@redhat.com)
- added readiness check to LDAP server pod (skuznets@redhat.com)
- diagnostics: avoid some redundancy (lmeyer@redhat.com)
- update auth tests to use actual master (deads@redhat.com)
- fix non-compliant build integration tests (deads@redhat.com)
- Wait until animation finishes to call chart.flush() (spadgett@redhat.com)
- oc: Add more doc and examples in oc get (mkargaki@redhat.com)
- Make KUBE_TIMEOUT take a duration (rhcarvalho@gmail.com)
- examples: Update resource quota README (mkargaki@redhat.com)
- promoted group prune and sync from experimental (skuznets@redhat.com)
- Various accessibility fixes, bumps angular-bootstrap version
  (jforrest@redhat.com)
- Avoid scrollbar flicker on build trends tooltip (spadgett@redhat.com)
- Show empty RCs in some cases on overview when no service
  (spadgett@redhat.com)
- make os::cmd::try_until* output smarter (skuznets@redhat.com)
- Fix tito ldflag manipulation at tag time (sdodson@redhat.com)
- graphapi: Remove dead code and add godoc (mkargaki@redhat.com)
- describe DockerBuildStrategy.DockerfilePath (v.behar@free.fr)
- integration-tests: retry get image on not found error (miminar@redhat.com)
- Wait for user permissions in test-cmd.sh (jliggitt@redhat.com)
- Wait for bootstrap policy on startup (jliggitt@redhat.com)
- Shorten image importer dialTimeout to 5 seconds (jliggitt@redhat.com)
- Increase specificity of CSS .yaml-mode .ace-numeric style
  (spadgett@redhat.com)
- Fix typo in example `oc run` command. (dusty@dustymabe.com)
- deployapi: Necessary refactoring after updating the internal objects
  (mkargaki@redhat.com)
- deployapi: Update generated conversions and deep copies (mkargaki@redhat.com)
- deployapi: Update manual conversions (mkargaki@redhat.com)
- deployapi: Refactor internal objects to match versioned (mkargaki@redhat.com)
- Allow using an image as source for a build (cewong@redhat.com)
- Updating after real world tests (tdawson@redhat.com)
- added os::cmd readme (skuznets@redhat.com)
- fixed caching bug in ldap sync (skuznets@redhat.com)
- update deltafifo usage to match upstream changes (bparees@redhat.com)
- UPSTREAM: 14881: fix delta fifo & various fakes for go1.5.1
  (maszulik@redhat.com)

* Sat Dec 19 2015 Scott Dodson <sdodson@redhat.com> 3.1.1.0
- Fix tito ldflag manipulation at tag time (sdodson@redhat.com)
- Fix oc status unit test (mkargaki@redhat.com)
- Improve web console scaling (spadgett@redhat.com)
- UPSTREAM: <drop>: fixup for 14537 (mturansk@redhat.com)
- Add bash auto-completion for oc, oadm, and openshift to the environments
  created by Vagrant. (bbennett@redhat.com)
- Fix github link when contextDir is set but not git ref (jforrest@redhat.com)
- install which in the base image (bparees@redhat.com)
- UPSTREAM: 18165: fixes get --show-all (ffranz@redhat.com)
- fix extra indentation for bare builds (deads@redhat.com)
- Edit resource YAML in the web console (spadgett@redhat.com)
- Use DeepEqual instead of field by field comparison (rhcarvalho@gmail.com)
- Fix regression: new-app with custom Git ref (rhcarvalho@gmail.com)
- Increase debug logging for image import controller (jliggitt@redhat.com)
- Gather logs for test-cmd (jliggitt@redhat.com)
- UPSTREAM: 18621: Implement GCE PD dynamic provisioner. (jsafrane@redhat.com)
- UPSTREAM: 17747: Implement GCE PD disk creation. (jsafrane@redhat.com)
- UPSTREAM: 18607: Implement OpenStack Cinder dynamic provisioner.
  (jsafrane@redhat.com)
- UPSTREAM: 18601: Implement AWS EBS dynamic provisioner. (jsafrane@redhat.com)
- Fix test-cmd.sh on OSX (jliggitt@redhat.com)
- Web console: Fix edge case scaling to 0 replicas (spadgett@redhat.com)
- correctly determine build pushability for status (deads@redhat.com)
- move imagestream creation to more sensible spot (bparees@redhat.com)
- Avoid generating tokens with leading dashes (jliggitt@redhat.com)
- Show 'none' when there is no builder image. (rhcarvalho@gmail.com)
- status: Add more details for a broken dc trigger (mkargaki@redhat.com)
- diagnose timeouts correctly in error message (skuznets@redhat.com)
- Fix up ClusterNetwork validation. (danw@redhat.com)
- Show warning when --env is used in start-build with binary source
  (mfojtik@redhat.com)
- Fix deployements/stragies anchor link (christophe@augello.be)
- don't output empty strings in logs (bparees@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  e7f0d8be285f73c896ff19455bd03d5189cbe5e6 (dcbw@redhat.com)
- Fix race between Kubelet initialization and plugin creation (dcbw@redhat.com)
- Output more helpful cancel message (jliggitt@redhat.com)
- Use the iptables-based proxier instead of the userland one (danw@redhat.com)
- Delete tag if its history is empty (miminar@redhat.com)
- Remove redundant admin routes (miminar@redhat.com)
- Reenabled registry dc's liveness probe (miminar@redhat.com)
- Refactor of OSO's code (miminar@redhat.com)
- Refactor dockerregistry: prevent a warning during startup
  (miminar@redhat.com)
- Refactor dockerregistry: unify logging style with upstream's codebase
  (miminar@redhat.com)
- Refactor dockerregistry: use registry's own health check (miminar@redhat.com)
- Refactor dockerregistry: adapt to upstream changes (miminar@redhat.com)
- Registry refactor: handle REGISTRY_CONFIGURATION_PATH (miminar@redhat.com)
- unbump(code.google.com/p/go-uuid/uuid): which is obsoleted
  (miminar@redhat.com)
- Always use port 53 as the dns service port. (abutcher@redhat.com)
- Fix service link on route page (spadgett@redhat.com)
- Increase contrast of web console log text (spadgett@redhat.com)
- custom Dockerfile path for the docker build (v.behar@free.fr)
- assets: Fix up the topology view icon colors and lines (stefw@redhat.com)
- Remove static nodes, deprecate --nodes (jliggitt@redhat.com)
- Add PV Provisioner Controller (mturansk@redhat.com)
- Fix for bugz https://bugzilla.redhat.com/show_bug.cgi?id=1290643   o Make
  Forwarded header value rfc7239 compliant.   o Set X-Forwarded-Proto for http
  (if insecure edge terminated routes     are allowed). (smitram@gmail.com)
- UPSTREAM: 14537: Add PersistentVolumeProvisionerController
  (mturansk@redhat.com)
- Fix test failures for scoped and host-overriden routers. (smitram@gmail.com)
- Unit test for hostname override (ccoleman@redhat.com)
- Routers should be able to override the host value on Routes
  (ccoleman@redhat.com)
- added support for recursive testing using test-go (skuznets@redhat.com)
- fixed group detection bug for LDAP prune (skuznets@redhat.com)
- removed tryuntil from test-cmd (skuznets@redhat.com)
- fixed LDAP test file extensions (skuznets@redhat.com)
- Allow image importing to work with proxy (jkhelil@gmail.com)
- oc: Use object name instead of provided arg in cancel-build
  (mkargaki@redhat.com)
- add step for getting the jenkins service ip (bparees@redhat.com)
- fix forcepull setup build config (gmontero@redhat.com)
- Add user/group reapers (jliggitt@redhat.com)
- Make project template use default rolebinding name (jliggitt@redhat.com)
- update missing imagestream message to warning (bparees@redhat.com)
- Temporary fix for systemd upgrade path issues (sdodson@redhat.com)
- Add logging for etcd integration tests (jliggitt@redhat.com)
- Fix the mysql replica extended test (mfojtik@redhat.com)
- Use service port name as route targetPort in 'oc expose service'
  (jliggitt@redhat.com)
- Refactor SCM auth and use of env vars in builders (cewong@redhat.com)
- oc: Cosmetic fixes in oc status (mkargaki@redhat.com)
- allow for test specification from command-line for test-cmd
  (skuznets@redhat.com)
- Change uuid imports to github.com/pborman/uuid (miminar@redhat.com)
- UPSTREAM: docker/distribution: <carry>: remove parents on delete
  (miminar@redhat.com)
- UPSTREAM: docker/distribution: <carry>: export app.Namespace
  (miminar@redhat.com)
- UPSTREAM: docker/distribution: <carry>: custom routes/auth
  (agoldste@redhat.com)
- UPSTREAM: docker/distribution: 1050: Exported API functions needed for
  pruning (miminar@redhat.com)
- bump(github.com/stevvooe/resumable): 51ad44105773cafcbe91927f70ac68e1bf78f8b4
  (miminar@redhat.com)
- bump(github.com/docker/distribution):
  e6c60e79c570f97ef36f280fcebed497682a5f37 (miminar@redhat.com)
- Give user suggestion about new-app on new-project (ccoleman@redhat.com)
- Controllers should always go async on start (ccoleman@redhat.com)
- Clean up docker-in-docker image (marun@redhat.com)
- doc: create glusterfs service to persist endpoints (hchen@redhat.com)
- [RPMs] Add requires on git (sdodson@redhat.com)
- added better output to test-end-to-end/core (skuznets@redhat.com)
- refactored scripts to use hack/text (skuznets@redhat.com)

* Mon Dec 14 2015 Troy Dawson <tdawson@redhat.com> 3.1.0.902
- Include LICENSE in client zips (ccoleman@redhat.com)
- Minor commit validation fixes (ironcladlou@gmail.com)
- remove build-related type fields from internal api (bparees@redhat.com)
- Add godeps commit verification (ironcladlou@gmail.com)
- Retry build logs in start build when waiting for build (mfojtik@redhat.com)
- Create proper client packages for mac and windows (ccoleman@redhat.com)
- update jenkins tutorial for using plugin (gmontero@redhat.com)
- Allow oc new-build to accept zero arguments (ccoleman@redhat.com)
- Fix fallback scaling behavior (ironcladlou@gmail.com)
- Fix deployment e2e flake (mkargaki@redhat.com)
- UPSTREAM: 18522: Close web socket watches correctly (jliggitt@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  8a7e17c0c3eea529955229dfd7b4baefad56633b (rpenta@redhat.com)
- Start SDN controller after running kubelet (rpenta@redhat.com)
- [RPMs] Cleanup kubeplugin path from old sdn-ovs installs (sdodson@redhat.com)
- Fix flakiness in builds extended tests (cewong@redhat.com)
- Add suggestions in oc status (mkargaki@redhat.com)
- status: Report tls routes with unspecified termination type
  (mkargaki@redhat.com)
- Use the dockerclient ClientFromEnv setup (ccoleman@redhat.com)
- Don't show build trends chart scrollbars if not needed (spadgett@redhat.com)
- Prevent y-axis label overlap when filtering build trends chart
  (spadgett@redhat.com)
- Packaging specfile clean up (admiller@redhat.com)
- added prune-groups; refactored rfc2307 ldapinterface (skuznets@redhat.com)
- only expand resources when strictly needed (deads@redhat.com)
- added logging of output for failed builds (ipalade@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  0f33df18b9747ebfe2c337f2bf4443b520a8f2ab (rpenta@redhat.com)
- UPSTREAM: revert: 97bd6c: <carry>: Allow pod start to be delayed in Kubelet
  (rpenta@redhat.com)
- Don't use KubeletConfig 'StartUpdates' channel for blocking kubelet
  (rpenta@redhat.com)
- after live test (tdawson@redhat.com)
- Unload filtered groups from build config chart (spadgett@redhat.com)
- Fix login tests for OSE variants (sdodson@redhat.com)
- Optionally skip builds for dev cluster provision (marun@redhat.com)
- Fix for bugz 1283952 and add a test. (smitram@gmail.com)
- more code (tdawson@redhat.com)
- more code (tdawson@redhat.com)
- Disable delete button for RCs with status replicas (spadgett@redhat.com)
- Web console support for deleting individual builds and deployments
  (spadgett@redhat.com)
- add new options (tdawson@redhat.com)
- UPSTREAM: 18065: Fixed forbidden window enforcement in horizontal pod
  autoscaler (sross@redhat.com)
- more updating of script (tdawson@redhat.com)
- first rough draft (tdawson@redhat.com)
- HACKING.md: clarify meaning and provide proper usage of OUTPUT_COVERAGE
  variable. (vsemushi@redhat.com)

* Tue Dec 08 2015 Scott Dodson <sdodson@redhat.com> 3.1.0.901
- Show build trends chart on build config page (spadgett@redhat.com)
- fix ruby-22 scl enablement (bparees@redhat.com)
- dump build logs on failure (bparees@redhat.com)
- make sure imagestreams are imported before using them with new-app
  (bparees@redhat.com)
- increase build timeout to 60mins (bparees@redhat.com)
- Update config of dind cluster image registry (marun@redhat.com)
- Allow junit output filename to be overridden (marun@redhat.com)
- improved os::cmd handling of test names, timing (skuznets@redhat.com)
- Improve deployment scaling behavior (ironcladlou@gmail.com)
- refactored test/cmd/admin to use wrapper functions (skuznets@redhat.com)
- Create sourcable bash env as part of dind cluster deploy (marun@redhat.com)

* Fri Dec 04 2015 Scott Dodson <sdodson@redhat.com> 3.1.0.900
- Update liveness/readiness probe to always use the /healthz endpoint (on the
  stats port or on port 1936 if the stats are disabled). (smitram@gmail.com)
- Fix incorrect status icon on pods page (spadgett@redhat.com)
- Update rest of controllers to use new ProjectsService controller, remove ng-
  controller directives in templates, all controllers defined in routes, minor
  update to tests (admin@benjaminapetersen.me)
- Remove the tooltip from the delete button (spadgett@redhat.com)
- Add success line to dry run (rhcarvalho@gmail.com)
- Add integration test for router healthz endpoint as per @smarterclayton
  review comments. (smitram@gmail.com)
- UPSTREAM: drop: part of upstream kube PR #15843. (avagarwa@redhat.com)
- UPSTREAM: drop: Fix kube e2e tests in origin. This commit is part of upstream
  kube PR 16360. (avagarwa@redhat.com)
- Bug 1285626 - need to handle IS tag with a from of kind DockerImage
  (jforrest@redhat.com)
- Fix for Bug 1285647 and issue 6025  - Border width will be expanded when long
  value added in Environment Variables for project  - Route link on overview is
  truncated even at wide browser widths (sgoodwin@redhat.com)
- Patch AOS tuned-profiles manpage during build (sdodson@redhat.com)
- Do not print steps for alternative output formats (rhcarvalho@gmail.com)
- UPSTREAM: 17920: Fix frequent kubernetes endpoint updates during cluster
  start (abutcher@redhat.com)
- Add additional route settings to UI (spadgett@redhat.com)
- switch hello-world tests to expect ruby-22 (bparees@redhat.com)
- Upstream: 16728: lengthened pv controller sync period to 10m
  (mturansk@redhat.com)
- Support deleting routes in web console (spadgett@redhat.com)
- oc rsync: pass-thru global command line options (cewong@redhat.com)
- Sync Dockerfile.product from dist-git (sdodson@redhat.com)
- add namespace to field selectors (pweil@redhat.com)
- Use minutes instead of seconds where possible. (vsemushi@redhat.com)
- added volume length check for overriden recycler (mturansk@redhat.com)
- make adding infrastructure SAs easier (deads@redhat.com)
- fixed origin recycler volume config w/ upstream cli flags
  (mturansk@redhat.com)
- oc: Make env use PATCH instead of PUT (mkargaki@redhat.com)
- Prompt before scaling deployments to 0 in UI (spadgett@redhat.com)
- UPSTREAM: 18000: Fix test failure due to days-in-month check. Issue #17998.
  (mkargaki@redhat.com)
- Update install for osdn plugin reorg (danw@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  0d3440e224aeb26a056c0c4c91c30fdbb59588f9 (danw@redhat.com)
- Update various templates to use status-icon (admin@benjaminapetersen.me)
- UPSTREAM: 17973: Validate pod spec.nodeName (jliggitt@redhat.com)
- Rename project service to ProjectsService, update all pre-existing occurances
  (admin@benjaminapetersen.me)
- Hide old deployments in the topology view (spadgett@redhat.com)
- refactored constructors to allow for better code resue with prune-groups
  (skuznets@redhat.com)
- Prevent identical input/output IST in oc new-build (rhcarvalho@gmail.com)
- Add aria-describedby attributes to template parameter inputs
  (spadgett@redhat.com)
- remove url workaround (now at correct s2i level) (gmontero@redhat.com)
- New flag to oc new-build to produce no output (rhcarvalho@gmail.com)
- Fix sr-only text for pod status chart (spadgett@redhat.com)
- Add README.md to examples/db-templates (rhcarvalho@gmail.com)
- Fix test registry resource location (ffranz@redhat.com)
- UPSTREAM: 17886: pod log location must validate container if provided
  (ffranz@redhat.com)
- Background the node service so we handle SIGTERM (sdodson@redhat.com)
- hack/util.sh(delete_large_and_empty_logs): optimize find usage.
  (vsemushi@redhat.com)
- Bug 1281928 - fix image stream tagging for DockerImage type images.
  (maszulik@redhat.com)
- Remove type conversion (rhcarvalho@gmail.com)
- Bug1277420 - show friendly prompt when cancelling a completed build
  (jhadvig@redhat.com)
- Add new-build flag to set output image reference (rhcarvalho@gmail.com)
- Output uppercase, hex, 2-character padded serial.txt (jliggitt@redhat.com)
- Fix test/cmd/admin.shwq (jliggitt@redhat.com)
- Fix template processing multiple values (jliggitt@redhat.com)
- CLI usability - proposed aliases (ffranz@redhat.com)
- overhaul imagestream definitions and update latest (bparees@redhat.com)
- refactored test/cmd/images to use wrapper methods (skuznets@redhat.com)
- added unit testing to existing LDAP sync code (skuznets@redhat.com)
- fix macro order for bundled listing in tito custom builder
  (admiller@redhat.com)
- refactor fedora packaging additions with tito custom builder updates
  (admiller@redhat.com)
- Fedora packaging: (admiller@redhat.com)
- Typo fixes (jhadvig@redhat.com)
- Update templates to use navigateResourceURL filter where appropriate
  (admin@benjaminapetersen.me)
- Update completions (ffranz@redhat.com)
- bump(github.com/spf13/pflag): 08b1a584251b5b62f458943640fc8ebd4d50aaa5
  (ffranz@redhat.com)
- bump(github.com/spf13/cobra): 1c44ec8d3f1552cac48999f9306da23c4d8a288b
  (ffranz@redhat.com)
- Adds .docker/config.json secret example (ffranz@redhat.com)
- refactored test/cmd/basicresources to use wrapper functions
  (skuznets@redhat.com)
- added os::cmd::try_until* (skuznets@redhat.com)
- no redistributable for either fedora or epel (tdawson@redhat.com)
- Skip Daemonset and DaemonRestart as these are not enabled yet and keep
  failing. (avagarwa@redhat.com)
- Retry failed attempts to talk to remote registry (miminar@redhat.com)
- Split UI tests into e2e vs rest_api integration suites (jforrest@redhat.com)
- refactored test/cmd/templates to use wrapper methods (skuznets@redhat.com)
- refactored test/cmd/policy to use wrapper methods (skuznets@redhat.com)
- refactored test/cmd/builds to use wrapper methods (skuznets@redhat.com)
- refactored test/cmd/newapp to use wrapper functions (skuznets@redhat.com)
- refactored test/cmd/help to use wrapper functions (skuznets@redhat.com)
- avoid: no such file error in test-cmd (deads@redhat.com)
- added ldap test client and query tests (skuznets@redhat.com)
- Link in the alert for the newly triggered build (jhadvig@redhat.com)
- reorganized ldaputil error and query code (skuznets@redhat.com)
- refactored test/cmd/export to use wrapper functions (skuznets@redhat.com)
- refactored test/cmd/deployments to use wrapper methods (skuznets@redhat.com)
- refactored test/cmd/volumes to use helper methods (skuznets@redhat.com)
- Update build revision information when building (cewong@redhat.com)
- Enable /healthz irrespective of stats port being enabled/disabled. /healthz
  is available on the stats port or the default stats port 1936 (if stats are
  turned off via --stats-port=0). (smitram@gmail.com)
- refactored test/cmd/secrets to use helper methods (skuznets@redhat.com)
- added cmd util function test to CI (skuznets@redhat.com)
- bump(gopkg.in/asn1-ber.v1): 4e86f4367175e39f69d9358a5f17b4dda270378d
  (jliggitt@redhat.com)
- bump(gopkg.in/ldap.v2): e9a325d64989e2844be629682cb085d2c58eef8d
  (jliggitt@redhat.com)
- Rename github.com/go-ldap/ldap to gopkg.in/ldap.v2 (jliggitt@redhat.com)
- bump(gopkg.in/ldap.v2): b4c9518ccf0d85087c925e4a3c9d5802c9bc7025 (package
  rename) (jliggitt@redhat.com)
- add role reaper (deads@redhat.com)
- Exposes the --token flag in login command help (ffranz@redhat.com)
- allow unknown secret types (deads@redhat.com)
- allow startup of API server only for integration tests (deads@redhat.com)
- bump(github.com/openshift/source-to-image)
  7597eaa168a670767bf2b271035d29b92ab13b5c (cewong@redhat.com)
- refactored hack/test to not use aliases, detect tty (skuznets@redhat.com)
- Refactor pkg/generate/app (rhcarvalho@gmail.com)
- add image-pusher role (deads@redhat.com)
- refactored test-cmd/edit to use new helper methods (skuznets@redhat.com)
- remove db tag from jenkins template (bparees@redhat.com)
- Change default instance size (dmcphers@redhat.com)
- bump the wait for images timeout (bparees@redhat.com)
- dump container logs when s2i incremental tests fail (bparees@redhat.com)
- test non-db sample templates also (bparees@redhat.com)
- Use correct homedir on Windows (ffranz@redhat.com)
- UPSTREAM: 17590: correct homedir on windows (ffranz@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  919e0142fe594ab5115ecf7fa3f7ad4f5810f009 (dcbw@redhat.com)
- Update to new osdn plugin API (dcbw@redhat.com)
- Accept CamelCase versions of TLS config (ccoleman@redhat.com)
- fix git-ls to leverage GIT_SSH for non-git, secret/token access
  (gmontero@redhat.com)
- Bug 1277046 - fixed tagging ImageStreamImage from the same ImageStream to
  point to original pull spec instead of the internal registry.
  (maszulik@redhat.com)
- Get rid of util.StringList (ffranz@redhat.com)
- UPSTREAM: revert: 199adb7: <drop>: add back flag types to reduce noise during
  this rebase (ffranz@redhat.com)
- added start-build parameters to cli.md (ipalade@redhat.com)
- UPSTREAM: 17567: handle the HEAD verb correctly for authorization
  (deads@redhat.com)
- examples/sample-app/README.md: fix commands to simplify user experience.
  (vsemushi@redhat.com)
- Add PostgreSQL replication tests (nagy.martin@gmail.com)
- Allow to override environment and build log level in oc start-build
  (mfojtik@redhat.com)
- Remove hook directory by default in gitserver example (cewong@redhat.com)
- wait for imagestream import before running a build in tests
  (bparees@redhat.com)
- extended tests for docker and sti bc with no outputname defined
  (ipalade@redhat.com)
- The git:// protocol with proxy is not allowed (mfojtik@redhat.com)
- Fix build controller integration test flake (cewong@redhat.com)
- add requester username to project template (deads@redhat.com)
- Fix extended test for start-build (mfojtik@redhat.com)
- prevent go panic when output not specified for build config
  (gmontero@redhat.com)
- fix start-build --from-webhook (cewong@redhat.com)
- SCMAuth: use local proxy when password length exceeds 255 chars
  (cewong@redhat.com)
- Use constants for defaults instead of strings (rhcarvalho@gmail.com)
- Add extended test for proxy (mfojtik@redhat.com)
- Cleanup extended test output some more (nagy.martin@gmail.com)
- Use router's /healtz route for health checks (miminar@redhat.com)
- Unflake MySQL extended test (nagy.martin@gmail.com)
- Add git ls-remote to validate the remote GIT repository (mfojtik@redhat.com)
- Handle openshift.io/build-config.name label as well as buildconfig.
  (vsemushi@redhat.com)
- update the readme (haoran@dhcp-129-204.nay.redhat.com)
- Completions for persistent flags (ffranz@redhat.com)
- UPSTREAM: spf13/cobra 180: fixes persistent flags completions
  (ffranz@redhat.com)
- Generated docs must use the short command name (ffranz@redhat.com)
- UPSTREAM: 17033: Fix default value for StreamingConnectionIdleTimeout
  (avagarwa@redhat.com)
- Fixes bug 1275518 https://bugzilla.redhat.com/show_bug.cgi?id=1275518
  (avagarwa@redhat.com)
- tito builder/tagger cleanup: (admiller@redhat.com)
- stop oc export from exporting SA secrets that aren't round-trippable
  (deads@redhat.com)
- accept new dockercfg format (deads@redhat.com)
- Fix alignment of icon on settings page by overriding patternfly rule
  (sgoodwin@redhat.com)
- spec: Use relative symlinks in bin/ (walters@verbum.org)
- Added readiness probe for Router (miminar@redhat.com)
- Add openshift/origin-gitserver to push-release.sh (cewong@redhat.com)
- To replace Func postfixed identifier with Fn postfixed in e2e extended
  (salvatore-dario.minonne@amadeus.com)
- to add a job test to extended test (salvatore-dario.minonne@amadeus.com)
- Allow kubelet to be configured for dind compat (marun@redhat.com)
- Force network tests to wait until cluster is ready (marun@redhat.com)
- Update dind docs to configure more vagrant memory (marun@redhat.com)
- Run networking sanity checks separately. (marun@redhat.com)
- Add 'redeploy' command to dind cluster script (marun@redhat.com)
- Fix networking extended test suite declaration (marun@redhat.com)
- Skip internet check in network extended test suite (marun@redhat.com)
- Skip sdn node during dev cluster deploy (marun@redhat.com)
- Fix handling of network plugin arg in deployment (marun@redhat.com)
- Ensure deltarpm is used for devcluster deployment. (marun@redhat.com)
- Rename OPENSHIFT_SDN env var (marun@redhat.com)
- Refactor extended networking test script (marun@redhat.com)
- Increase verbosity of networking test setup (marun@redhat.com)
- Deploy ssh by default on dind cluster nodes (marun@redhat.com)
- Fix numeric comparison bug in cluster provisioning (marun@redhat.com)
- Make provisioning output less noisy (marun@redhat.com)
- Retain systemd logs from extended networking tests (marun@redhat.com)
- Allow networking tests to target existing cluster (marun@redhat.com)
- Optionally skip builds during cluster provision (marun@redhat.com)
- Enable parallel dev cluster deployment (marun@redhat.com)
- Disable scheduling for sdn node when provisioning (marun@redhat.com)
- Rename bash functions used for provisioning (marun@redhat.com)
- Doc and env var cleanup for dind refactor (marun@redhat.com)
- Switch docker-in-docker to use systemd (marun@redhat.com)
- fix merge conflicts in filters/resources.js, update bindata
  (gabriel_ruiz@symantec.com)
- add tests for variable expansion (bparees@redhat.com)
- Use assets/config.local.js if present for development config
  (spadgett@redhat.com)
- Fix typos (dmcphers@redhat.com)
- Allow tag already exist when pushing a release (ccoleman@redhat.com)
- Fixes attach example (ffranz@redhat.com)
- UPSTREAM: 17239: debug filepath in config loader (ffranz@redhat.com)
- Allow setting build config environment variables, show env vars on build
  config page (jforrest@redhat.com)
- UPSTREAM: 17236: fixes attach example (ffranz@redhat.com)
- Remove failing Docker Registry client test (ccoleman@redhat.com)
- leverage new source-to-image API around git clone spec validation/correction
  (gmontero@redhat.com)
- Reload proxy rules on firewalld restart, etc (danw@redhat.com)
- Fix serviceaccount in gitserver example yaml (cewong@redhat.com)
- gitserver: return appropriate error when auth fails (cewong@redhat.com)
- provide validation for build source type (bparees@redhat.com)
- Show less output in test-cmd.sh (ccoleman@redhat.com)
- Change the image workdir to be /var/lib/origin (ccoleman@redhat.com)
- bump(github.com/openshift/source-to-image):
  c9985b5443c4a0a0ffb38b3478031dcc2dc8638d (gmontero@redhat.com)
- Add deployment logs to UI (spadgett@redhat.com)
- Push recycler image (jliggitt@redhat.com)
- Prevent sending username containing colon via basic auth
  (jliggitt@redhat.com)
- added test-cmd test wrapper functions and tests (skuznets@redhat.com)
- Dont loop over all the builds / deployments for the config when we get an
  update for one (jforrest@redhat.com)
- Avoid mobile Safari zoom on input focus (spadgett@redhat.com)
- add build pod name annotation to builds (bparees@redhat.com)
- Prevent autocorrect and autocapitilization for some inputs
  (spadgett@redhat.com)
- bump rails test retry timeout (bparees@redhat.com)
- Adding the recycle tool to the specfile (bleanhar@redhat.com)
- Prevent route from overflowing box in mobile Safari (spadgett@redhat.com)
- Update recycler image to use binary (jliggitt@redhat.com)
- bump(go-ldap/ldap): b4c9518ccf0d85087c925e4a3c9d5802c9bc7025
  (skuznets@redhat.com)
- status: Warn about transient deployment trigger errors (mkargaki@redhat.com)
- Updated pv recycler to work with uid:gid (mturansk@redhat.com)
- Set registry service's session affinity (miminar@redhat.com)
- Several auto-completion fixes (ffranz@redhat.com)
- Fixes auto-completion for build config names (ffranz@redhat.com)
- show warning when pod's containers are restarting (deads@redhat.com)
- Change how we store association between builds and ICTs on DCs for the
  console overview (jforrest@redhat.com)
- Point docs at docs.openshift.com/enterprise/latest (sdodson@redhat.com)
- eliminate double bc reporting in status (deads@redhat.com)
- Use a different donut color for pods not ready (spadgett@redhat.com)
- disable --validate flag by default (deads@redhat.com)
- Update CONTRIBUTING.adoc (bpeterse@redhat.com)
- Show deployment status on overview when failed or cancelled
  (spadgett@redhat.com)
- UPSTREAM: revert: 0048df4: <carry>: Disable --validate by default
  (deads@redhat.com)
- add reconcile-cluster-role arg for specifying specific roles
  (deads@redhat.com)
- Reduce number of tick labels in metrics sparkline (spadgett@redhat.com)
- fix openshift client cache for different versions (deads@redhat.com)
- UPSTREAM: 17058: fix client cache for different versions (deads@redhat.com)
- make export-all work in failures (deads@redhat.com)
- Fix build-waiting logic to use polling instead of watcher
  (nagy.martin@gmail.com)
- add APIGroup to role describer (deads@redhat.com)
- UPSTREAM: 17017: stop jsonpath panicing on bad array length
  (deads@redhat.com)
- don't update the build phase once it reaches a terminal state
  (bparees@redhat.com)
- Run registry as non-root user (ironcladlou@gmail.com)
- warn on missing log and metric URLs for console (deads@redhat.com)
- don't show context nicknames that users don't recognize (deads@redhat.com)
- Allow non-alphabetic characters in expression generator (mfojtik@redhat.com)
- WIP - try out upstream e2e (ccoleman@redhat.com)
- add a wordpress template (bparees@redhat.com)
- UPSTREAM: 16945: kubelet: Fallback to api server for pod status
  (mkargaki@redhat.com)
- Bug 1278007 - Use commit instead of HEAD when streaming in start-build
  (mfojtik@redhat.com)
- Move os::util:install-sdn to contrib/node/install-sdn.sh (sdodson@redhat.com)
- Drop selinux relabeling for volumes, add images to push-release
  (sdodson@redhat.com)
- Build controller - set build status only if pod creation succeeds
  (cewong@redhat.com)
- Let users to select log content without line numbers (spadgett@redhat.com)
- pre-set the SA namespaces (deads@redhat.com)
- Add labels and annotations to DeploymentStrategy. (roque@juniper.net)
- containerized-installs -> containerized (sdodson@redhat.com)
- Add example systemd units for running as container (sdodson@redhat.com)
- Add sdn ovs enabled node image (sdodson@redhat.com)
- fix case for events (deads@redhat.com)
- prune: Remove deployer pods when pruning failed deployments
  (mkargaki@redhat.com)
- reenable static building of hello pod (bparees@redhat.com)
- do not make redistributable in fedora (tdawson@redhat.com)
- Make building clients for other architectures optional (tdawson@redhat.com)

* Tue Nov 10 2015 Scott Dodson <sdodson@redhat.com> 3.1.0.4
- change OS bootstrap SCCs to use RunAsAny for fsgroup and sup groups
  (pweil@redhat.com)
- UPSTREAM:<carry>:v1beta3 default fsgroup/supgroup strategies to RunAsAny
  (pweil@redhat.com)
- UPSTREAM:<carry>:default fsgroup/supgroup strategies to RunAsAny
  (pweil@redhat.com)
- UPSTREAM: 17061: Unnecessary updates to ResourceQuota when doing UPDATE
  (decarr@redhat.com)
- Fix typo (dmcphers@redhat.com)
- Update HPA bootstrap policy (sross@redhat.com)
- Run deployer as non-root user (ironcladlou@gmail.com)
- UPSTREAM: 15537: openstack: cache InstanceID and use it for volume
  management. (jsafrane@redhat.com)

* Mon Nov 09 2015 Troy Dawson <tdawson@redhat.com> 3.1.0.3
- Add jenkins status to readme (dmcphers@redhat.com)
- cAdvisor needs access to dmsetup for devicemapper info (ccoleman@redhat.com)
- Make auth-in-container tests cause test failure again (ccoleman@redhat.com)
- UPSTREAM: 16969: nsenter file writer mangles newlines (ccoleman@redhat.com)
- Doc fixes (mkargaki@redhat.com)
- Doc fixes (dmcphers@redhat.com)
- Specify scheme/port for metrics client (jliggitt@redhat.com)
- UPSTREAM: 16926: Enable specifying scheme/port for metrics client
  (jliggitt@redhat.com)
- Test template preservation of integers (jliggitt@redhat.com)
- UPSTREAM: 16964: Preserve int64 data when unmarshaling (jliggitt@redhat.com)
- Given we don't restrict on Travis success.  Make what it does report be 100%%
  reliable and fast.  So when it does fail we know something is truly wrong.
  (dmcphers@redhat.com)
- back project cache with local authorizer (deads@redhat.com)

* Sat Nov 07 2015 Brenton Leanhardt <bleanhar@redhat.com> 3.1.0.2
- bump(github.com/openshift/openshift-sdn)
  d5965ee039bb85c5ec9ef7f455a8c03ac0ff0214 (dcbw@redhat.com)
- Identify the upstream Kube tag more clearly (ccoleman@redhat.com)
- Move etcd.log out of etcd dir (jliggitt@redhat.com)
- Conditionally run extensions controllers (jliggitt@redhat.com)
- Make namespace delete trigger exp resource delete (sross@redhat.com)

* Fri Nov 06 2015 Troy Dawson <tdawson@redhat.com> 3.1.0.1
- Allow in-cluster config for oc (ccoleman@redhat.com)
- deprecation for buildconfig label (bparees@redhat.com)
- Unable to submit subject rolebindings to a v1 server (ccoleman@redhat.com)
- Allow a service account installation (ccoleman@redhat.com)
- UPSTREAM: 16818: Namespace controller should always get latest state prior to
  deletion (decarr@redhat.com)
- UPSTREAM: 16859: Return a typed error for no-config (ccoleman@redhat.com)
- New-app: Set context directory and strategy on source repos specified with
  tilde(~) (cewong@redhat.com)
- update UPGRADE.md (pweil@redhat.com)

* Wed Nov 04 2015 Scott Dodson <sdodson@redhat.com> 3.1.0.0
- add reconcile-sccs command (pweil@redhat.com)
- Guard against servers that return non-json for the /v2/ check
  (ccoleman@redhat.com)
- Added PVController service account (mturansk@redhat.com)
- Disable deployment config detail message updates (ironcladlou@gmail.com)
- updates via github discussions (admin@benjaminapetersen.me)
- UPSTREAM: 16432: fixed pv binder race condition (mturansk@redhat.com)
- Fixes completions (ffranz@redhat.com)
- UPSTREAM: spf13/cobra: fixes filename completion (ffranz@redhat.com)
- Revert 7fc8ab5b2696b533e6ac5bea003e5a0622bdbf58 (jordan@liggitt.net)
- Automatic commit of package [atomic-openshift] release [3.0.2.906].
  (tdawson@redhat.com)
- UPSTREAM: 16384: Large memory allocation with key prefix generation
  (ccoleman@redhat.com)
- Update bash completions (decarr@redhat.com)
- UPSTREAM: 16749: Kubelet serialize image pulls had incorrect default
  (decarr@redhat.com)
- UPSTREAM: 15914: make kubelet images pulls serialized by default
  (decarr@redhat.com)
- New kibanna archive log link on log tab for build & pod
  (admin@benjaminapetersen.me)
- Transfer ImagePullSecrets to deployment hook pods (ironcladlou@gmail.com)
- Copy volume mounts to hook pods (ironcladlou@gmail.com)
- UPSTREAM: 16717: Ensure HPA has valid resource/name/subresource, validate
  path segments (jliggitt@redhat.com)
- Switch back to subnet (dmcphers@redhat.com)
- Inline deployer hook logs (ironcladlou@gmail.com)
- Remove default subnet (dmcphers@redhat.com)
- Disable quay.io test (ccoleman@redhat.com)
- UPSTREAM: 16032: revert origin 03e50db: check if /sbin/mount.nfs is present
  (mturansk@redhat.com)
- UPSTREAM: 16277: Fixed resetting last scale time in HPA status
  (sross@redhat.com)
- Change subnet default (dmcphers@redhat.com)
- Add default subnet back (dmcphers@redhat.com)
- Remove default subnet (dmcphers@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  cb0e352cd7591ace30d592d4f82685d2bcd38a04 (rpenta@redhat.com)
- Disable new-app Git tests in Vagrant (ccoleman@redhat.com)
- oc logs long description is wrong (ffranz@redhat.com)
- scc sort by priority (pweil@redhat.com)
- UPSTREAM:<carry>:v1beta3 scc priority field (pweil@redhat.com)
- UPSTREAM:<carry>:scc priority field (pweil@redhat.com)
- Bug and issue fixes (3) (sgoodwin@redhat.com)
- Fix deploy test conflict flake (ironcladlou@gmail.com)
- Prevent early exit on install-assets failure (jliggitt@redhat.com)
- UPSTREAM: 15997: Prevent NPE in resource printer on HPA (ccoleman@redhat.com)
- UPSTREAM: 16478: Daemon controller shouldn't place pods on not ready nodes
  (ccoleman@redhat.com)
- UPSTREAM: 16340: Kubelet pod status update is not correctly occuring
  (ccoleman@redhat.com)
- UPSTREAM: 16191: Mirror pods don't show logs (ccoleman@redhat.com)
- UPSTREAM: 14182: Distinguish image registry unavailable and pull failure
  (decarr@redhat.com)
- UPSTREAM: 16174: NPE when checking for mounting /etc/hosts
  (ccoleman@redhat.com)
-  Bug 1275537 - Fixed the way image import controller informs about errors
  from imports. (maszulik@redhat.com)
- UPSTREAM: 16052: Control /etc/hosts in the kubelet (ccoleman@redhat.com)
- UPSTREAM: 16044: Don't shadow error in cache.Store (ccoleman@redhat.com)
- UPSTREAM: 16025: Fix NPE in describe of HPA (ccoleman@redhat.com)
- UPSTREAM: 16668: Fix hpa escalation (deads@redhat.com)
- UPSTREAM: 15944: DaemonSet controller modifies the wrong fields
  (ccoleman@redhat.com)
- UPSTREAM: 15414: Annotations for kube-proxy move to beta
  (ccoleman@redhat.com)
- UPSTREAM: 15745: Endpoint timeouts in the proxy are bad (ccoleman@redhat.com)
- UPSTREAM: 15646: DaemonSet validation (ccoleman@redhat.com)
- UPSTREAM: 15574: Validation on resource quota (ccoleman@redhat.com)
- Calculate correct bottom scroll position (spadgett@redhat.com)
- oc tag should retry on conflict errors (ccoleman@redhat.com)
- Remove log arg for travis (jliggitt@redhat.com)
- hack/install-assets: fix nonstandard bash (lmeyer@redhat.com)
- extend role covers with groups (deads@redhat.com)
- Bug 1277021 - Fixed import-image help information. (maszulik@redhat.com)
- Deprecate build-logs in favor of logs (mkargaki@redhat.com)
- Build and deployment logs should check kubelet response (ccoleman@redhat.com)
- DNS services are not resolving properly (ccoleman@redhat.com)
- hack/test-end-to-end.sh won't start on IPv6 system (ccoleman@redhat.com)
- Wait longer for etcd startup in integration tests (ccoleman@redhat.com)
- Restore detailed checking for forbidden exec (jliggitt@redhat.com)
- UPSTREAM: 16711: Read error from failed upgrade attempts
  (jliggitt@redhat.com)
- Only show scroll links when log is offscreen (spadgett@redhat.com)
- Temporarily accept 'Forbidden' and 'forbidden' responses
  (jliggitt@redhat.com)
- Add HPA support for DeploymentConfig (sross@redhat.com)
- UPSTREAM: 16570: Fix GetRequestInfo subresource parsing for proxy/redirect
  verbs (sross@redhat.com)
- UPSTREAM: 16671: Customize HPA Heapster service namespace/name
  (sross@redhat.com)
- Add Scale Subresource to DeploymentConfigs (sross@redhat.com)
- bz 1276319 - Fix oc rsync deletion with tar strategy (cewong@redhat.com)
- UPSTREAM: <carry>: s/imagestraams/imagestreams/ in `oc get`
  (eparis@redhat.com)
- UPSTREAM(go-dockerclient): 408: fix stdin-only attach (agoldste@redhat.com)
- Add error clause to service/project.js (admin@benjaminapetersen.me)
- Switch to checking for CrashLoopBackOff to show the container looping message
  (jforrest@redhat.com)
- Fix namespace initialization (jliggitt@redhat.com)
- UPSTREAM: 16590: Create all streams before copying in exec/attach
  (agoldste@redhat.com)
- UPSTREAM: 16677: Add Validator for Scale Objects (sross@redhat.com)
- UPSTREAM: 16537: attach must only allow a tty when container supports it
  (ffranz@redhat.com)
- Bug 1276602 - fixes error when scaling dc with --timeout (ffranz@redhat.com)
- test/cmd/admin.sh isn't reentrant (ccoleman@redhat.com)
- add group/version serialization to master (deads@redhat.com)
- UPSTREAM: <drop>: allow specific, skewed group/versions (deads@redhat.com)
- UPSTREAM: 16667: Make Kubernetes HPA Controller use Namespacers
  (sross@redhat.com)
- rsync: output warnings to stdout instead of using glog (cewong@redhat.com)
- allow cluster-admin and cluster-reader to use different groups
  (deads@redhat.com)
- UPSTREAM: 16127: Bump cAdvisor (jimmidyson@gmail.com)
- UPSTREAM: 15612: Bump cadvisor (jimmidyson@gmail.com)
- rsync: output warnings to stdout instead of using glog (cewong@redhat.com)
- UPSTREAM: 16223: Concurrency fixes in kubelet status manager
  (ccoleman@redhat.com)
- UPSTREAM: 15275: Kubelet reacts much faster to unhealthy containers
  (ccoleman@redhat.com)
- UPSTREAM: 15706: HorizontalPodAutoscaler and Scale subresource APIs graduated
  to beta (sross@redhat.com)
- Fixed how tags are being printed when describing ImageStream. Previously the
  image.Spec.Tags was ignored, which resulted in not showing the tags for which
  there were errors during imports. (maszulik@redhat.com)

* Wed Nov 04 2015 Troy Dawson <tdawson@redhat.com> 3.0.2.906
- Copy volume mounts to hook pods (ironcladlou@gmail.com)
- UPSTREAM: 16717: Ensure HPA has valid resource/name/subresource, validate
  path segments (jliggitt@redhat.com)
- Switch back to subnet (dmcphers@redhat.com)
- Inline deployer hook logs (ironcladlou@gmail.com)
- Remove default subnet (dmcphers@redhat.com)
- Disable quay.io test (ccoleman@redhat.com)
- UPSTREAM: 16032: revert origin 03e50db: check if /sbin/mount.nfs is present
  (mturansk@redhat.com)
- UPSTREAM: 16277: Fixed resetting last scale time in HPA status
  (sross@redhat.com)
- Change subnet default (dmcphers@redhat.com)
- Add default subnet back (dmcphers@redhat.com)
- Remove default subnet (dmcphers@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  cb0e352cd7591ace30d592d4f82685d2bcd38a04 (rpenta@redhat.com)
- Disable new-app Git tests in Vagrant (ccoleman@redhat.com)
- oc logs long description is wrong (ffranz@redhat.com)
- scc sort by priority (pweil@redhat.com)
- UPSTREAM:<carry>:v1beta3 scc priority field (pweil@redhat.com)
- UPSTREAM:<carry>:scc priority field (pweil@redhat.com)
- Bug and issue fixes (3) (sgoodwin@redhat.com)
- Fix deploy test conflict flake (ironcladlou@gmail.com)
- Prevent early exit on install-assets failure (jliggitt@redhat.com)
- UPSTREAM: 15997: Prevent NPE in resource printer on HPA (ccoleman@redhat.com)
- UPSTREAM: 16478: Daemon controller shouldn't place pods on not ready nodes
  (ccoleman@redhat.com)
- UPSTREAM: 16340: Kubelet pod status update is not correctly occuring
  (ccoleman@redhat.com)
- UPSTREAM: 16191: Mirror pods don't show logs (ccoleman@redhat.com)
- UPSTREAM: 14182: Distinguish image registry unavailable and pull failure
  (decarr@redhat.com)
- UPSTREAM: 16174: NPE when checking for mounting /etc/hosts
  (ccoleman@redhat.com)
-  Bug 1275537 - Fixed the way image import controller informs about errors
  from imports. (maszulik@redhat.com)
- UPSTREAM: 16052: Control /etc/hosts in the kubelet (ccoleman@redhat.com)
- UPSTREAM: 16044: Don't shadow error in cache.Store (ccoleman@redhat.com)
- UPSTREAM: 16025: Fix NPE in describe of HPA (ccoleman@redhat.com)
- UPSTREAM: 16668: Fix hpa escalation (deads@redhat.com)
- UPSTREAM: 15944: DaemonSet controller modifies the wrong fields
  (ccoleman@redhat.com)
- UPSTREAM: 15414: Annotations for kube-proxy move to beta
  (ccoleman@redhat.com)
- UPSTREAM: 15745: Endpoint timeouts in the proxy are bad (ccoleman@redhat.com)
- UPSTREAM: 15646: DaemonSet validation (ccoleman@redhat.com)
- UPSTREAM: 15574: Validation on resource quota (ccoleman@redhat.com)
- Calculate correct bottom scroll position (spadgett@redhat.com)
- oc tag should retry on conflict errors (ccoleman@redhat.com)
- Remove log arg for travis (jliggitt@redhat.com)
- hack/install-assets: fix nonstandard bash (lmeyer@redhat.com)
- extend role covers with groups (deads@redhat.com)
- Bug 1277021 - Fixed import-image help information. (maszulik@redhat.com)
- Deprecate build-logs in favor of logs (mkargaki@redhat.com)
- Build and deployment logs should check kubelet response (ccoleman@redhat.com)
- DNS services are not resolving properly (ccoleman@redhat.com)
- hack/test-end-to-end.sh won't start on IPv6 system (ccoleman@redhat.com)
- Wait longer for etcd startup in integration tests (ccoleman@redhat.com)
- Restore detailed checking for forbidden exec (jliggitt@redhat.com)
- UPSTREAM: 16711: Read error from failed upgrade attempts
  (jliggitt@redhat.com)
- Only show scroll links when log is offscreen (spadgett@redhat.com)
- Temporarily accept 'Forbidden' and 'forbidden' responses
  (jliggitt@redhat.com)
- Add HPA support for DeploymentConfig (sross@redhat.com)
- UPSTREAM: 16570: Fix GetRequestInfo subresource parsing for proxy/redirect
  verbs (sross@redhat.com)
- UPSTREAM: 16671: Customize HPA Heapster service namespace/name
  (sross@redhat.com)
- Add Scale Subresource to DeploymentConfigs (sross@redhat.com)
- bz 1276319 - Fix oc rsync deletion with tar strategy (cewong@redhat.com)
- UPSTREAM: <carry>: s/imagestraams/imagestreams/ in `oc get`
  (eparis@redhat.com)
- UPSTREAM(go-dockerclient): 408: fix stdin-only attach (agoldste@redhat.com)
- Add error clause to service/project.js (admin@benjaminapetersen.me)
- Switch to checking for CrashLoopBackOff to show the container looping message
  (jforrest@redhat.com)
- Fix namespace initialization (jliggitt@redhat.com)
- UPSTREAM: 16590: Create all streams before copying in exec/attach
  (agoldste@redhat.com)
- UPSTREAM: 16677: Add Validator for Scale Objects (sross@redhat.com)
- UPSTREAM: 16537: attach must only allow a tty when container supports it
  (ffranz@redhat.com)
- Bug 1276602 - fixes error when scaling dc with --timeout (ffranz@redhat.com)
- test/cmd/admin.sh isn't reentrant (ccoleman@redhat.com)
- add group/version serialization to master (deads@redhat.com)
- UPSTREAM: <drop>: allow specific, skewed group/versions (deads@redhat.com)
- UPSTREAM: 16667: Make Kubernetes HPA Controller use Namespacers
  (sross@redhat.com)
- rsync: output warnings to stdout instead of using glog (cewong@redhat.com)
- Throttle log updates to avoid UI flicker (spadgett@redhat.com)
- allow cluster-admin and cluster-reader to use different groups
  (deads@redhat.com)
- UPSTREAM: 16127: Bump cAdvisor (jimmidyson@gmail.com)
- UPSTREAM: 15612: Bump cadvisor (jimmidyson@gmail.com)
- Bug 1277017 - Added checking if spec.DockerImageRepository and spec.Tags are
  not empty. (maszulik@redhat.com)
- rsync: output warnings to stdout instead of using glog (cewong@redhat.com)
- Bug 1276657 - Reuse --insecure-registry flag value when creating ImageStream
  from passed docker image. (maszulik@redhat.com)
- Use pficon-info on pod terminal tab (spadgett@redhat.com)
- Bug 1268000 - replace Image.DockerImageReference with value from status.
  (maszulik@redhat.com)
- UPSTREAM: 16223: Concurrency fixes in kubelet status manager
  (ccoleman@redhat.com)
- UPSTREAM: 15275: Kubelet reacts much faster to unhealthy containers
  (ccoleman@redhat.com)
- UPSTREAM: 16033: Mount returns verbose error (ccoleman@redhat.com)
- UPSTREAM: 16032: check if /sbin/mount.nfs is present (ccoleman@redhat.com)
- UPSTREAM: 15555: Use default port 3260 for iSCSI (ccoleman@redhat.com)
- UPSTREAM: 15562: iSCSI use global path to mount (ccoleman@redhat.com)
- UPSTREAM: 15236: Better error output from gluster (ccoleman@redhat.com)
- UPSTREAM: 15706: HorizontalPodAutoscaler and Scale subresource APIs graduated
  to beta (sross@redhat.com)
- Fixed how tags are being printed when describing ImageStream. Previously the
  image.Spec.Tags was ignored, which resulted in not showing the tags for which
  there were errors during imports. (maszulik@redhat.com)
- UPSTREAM: 15961: Add streaming subprotocol negotation (agoldste@redhat.com)
- Systemd throws an error on Restart=Always (sdodson@redhat.com)

* Sun Nov 01 2015 Scott Dodson <sdodson@redhat.com> 3.0.2.905
- Add and test field label conversions (jliggitt@redhat.com)
- logs: View logs from older deployments/builds with --version
  (mkargaki@redhat.com)
- UPSTREAM: 15733: Disable keepalive on liveness probes (ccoleman@redhat.com)
- UPSTREAM: 15845: Add service locator in service rest storage
  (ccoleman@redhat.com)
- install-assets: retry bower update (lmeyer@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  1f449c7f0d3cd41314a895ef119f9d25a15b54de (rpenta@redhat.com)
- Improve UI performance when displaying large logs (spadgett@redhat.com)
- fixes as per @smarterclayton review comments. (smitram@gmail.com)
- new SCCs (pweil@redhat.com)
- UPSTREAM: 16068: Increase annotation size significantly (ccoleman@redhat.com)
- UPSTREAM(go-dockerclient): 408: fix stdin-only attach (agoldste@redhat.com)
- stop creating roles with resourcegroups (deads@redhat.com)
- Fixes as per @smarterclayton and @liggit review comments. (smitram@gmail.com)
- Various style/positioning fixes (sgoodwin@redhat.com)
- Add missing kube resources to bootstrap policy (jliggitt@redhat.com)
- UPSTREAM: <carry>: OpenShift 3.0.2 nodes report v1.1.0-alpha
  (ccoleman@redhat.com)
- UPSTREAM: 16137: Release node port correctly (ccoleman@redhat.com)
- Run serialization tests for upstream types (mkargaki@redhat.com)
- UPSTREAM: <carry>: Update v1beta3 (mkargaki@redhat.com)
- UPSTREAM: 15930: Deletion of pods managed by old kubelets
  (ccoleman@redhat.com)
- UPSTREAM: 15900: Delete succeeded and failed pods immediately
  (ccoleman@redhat.com)
- add istag list, update (deads@redhat.com)
- diagnostics: default server conf paths changed (lmeyer@redhat.com)
- diagnostics: systemd unit name changes (lmeyer@redhat.com)
- Handle passwords with colon in basic auth (pep@redhat.com)
- Bug 1275564 - Removed the requirement for spec.dockerImageRepository from
  import-image command. (maszulik@redhat.com)
- bump(github.com/openshift/source-to-image)
  65d46436ab599633b76e570311a05f46a818389b (mfojtik@redhat.com)
- Add openshift.io/build-config.name label to builds. (vsemushi@redhat.com)
- oc: Use default resources where it makes sense (mkargaki@redhat.com)
- logs: Support all flags for builds and deployments (mkargaki@redhat.com)
- UPSTREAM: <carry>: Update v1beta3 PodLogOptions (mkargaki@redhat.com)
- UPSTREAM: 16494: Remove dead pods upon stopping a job (maszulik@redhat.com)
- Add local IP addresses to node certificate (jliggitt@redhat.com)
- Rest validation of binary builds is more aggressive (ccoleman@redhat.com)
-   o Add support to expose/redirect/disable insecure schemes (http) for
  edge secured routes.   o Add changes to template, haproxy and f5 router
  implementations.   o Add generated* files. (smitram@gmail.com)
- removed unneeded squash and chown from nfs doc (mturansk@redhat.com)
- Only run pod nodeenv admission on create (agoldste@redhat.com)
- fix up latest tags and add new scl image versions (bparees@redhat.com)
- UPSTREAM: 16532: Allow log tail and log follow to be specified together
  (ccoleman@redhat.com)
- Need to be doing a bower update instead of install so dependencies will
  update without conflict (jforrest@redhat.com)
- Update swagger spec (pmorie@gmail.com)
- UPSTREAM: 15799: Fix PodPhase issue caused by backoff (mkargaki@redhat.com)
- watchObject in console triggers callbacks for events of items of the same
  kind (jforrest@redhat.com)
- status: Report routes that have no route port specified (mkargaki@redhat.com)
- Add special casing for v1beta3 DeploymentConfig in serialization_test
  (pmorie@gmail.com)
- UPSTREAM: <carry>: respect fuzzing defaults for v1beta3 SecurityContext
  (pmorie@gmail.com)
- OS integration for PSC (pweil@redhat.com)
- UPSTREAM: <carry>: v1beta3 scc integration for PSC (pweil@redhat.com)
- UPSTREAM: <carry>: scc integration for PSC (pweil@redhat.com)
- UPSTREAM: <carry>: Workaround for cadvisor/libcontainer config schema
  mismatch (pmorie@gmail.com)
- UPSTREAM: 15323: Support volume relabling for pods which specify an SELinux
  label (pmorie@gmail.com)
- Update completions (jimmidyson@gmail.com)
- Test scm password auth (jliggitt@redhat.com)
- Add prometheus exporter to haproxy router (jimmidyson@gmail.com)
- UPSTREAM: 16332: Remove invalid blank line when printing jobs
  (maszulik@redhat.com)
- UPSTREAM: 16234: Fix jobs unittest flakes (maszulik@redhat.com)
- UPSTREAM: 16196: Fix e2e test flakes (maszulik@redhat.com)
- UPSTREAM: 15791: Update master service ports and type via controller.
  (abutcher@redhat.com)
- Add dns ports to the master service (abutcher@redhat.com)
- UPSTREAM: 15352: FSGroup implementation (pmorie@gmail.com)
- UPSTREAM: 14705: Inline some SecurityContext fields into PodSecurityContext
  (pmorie@gmail.com)
- UPSTREAM: 14991: Add Support for supplemental groups (pmorie@gmail.com)
- Allow processing template from different namespace (mfojtik@redhat.com)
- changed build for sti and docker to use OutputDockerImageReference, fixed
  tests (ipalade@redhat.com)
- Bug 1270728 - username in the secret don't override the username in the
  source URL (jhadvig@redhat.com)
- UPSTREAM: 15520: Move job to generalized label selector (maszulik@redhat.com)
- Update openshift-object-describer to 1.1.1 (jforrest@redhat.com)

* Thu Oct 29 2015 Scott Dodson <sdodson@redhat.com> 3.0.2.904
- Based on origin v1.0.7
- Provide informational output in new-app and new-build (ccoleman@redhat.com)
- Revert 'Retry ISM save upon conflicting IS error' commit upon retry mechanism
  introduced in ISM create method (maszulik@redhat.com)
- Bug 1275003: expose: Set route port based on service target port
  (mkargaki@redhat.com)
- Improve generation of name based on a git repo url to make it valid.
  (vsemushi@redhat.com)
- UPSTREAM: 16080: Convert from old mirror pods (1.0 to 1.1)
  (ccoleman@redhat.com)
- UPSTREAM: 15983: Store mirror pod hash in annotation (ccoleman@redhat.com)
- Slightly better output order in the CLI for login (ccoleman@redhat.com)
- Attempt to find merged parent (ccoleman@redhat.com)
- Fix extended tests for --from-* binary (ccoleman@redhat.com)
- Completions (ffranz@redhat.com)
- UPSTREAM: 16482: stdin is not a file extension for bash completions
  (ffranz@redhat.com)
- Bump angular-pattern to version 2.3.4 (spadgett@redhat.com)
- Use cmdutil.PrintSuccess to print objects (ffranz@redhat.com)
- Improve the error messages when something isn't found (ccoleman@redhat.com)
- UPSTREAM: 16445: Capitalize and expand UsageError message
  (ccoleman@redhat.com)
- Updates to address several issues (sgoodwin@redhat.com)
- Refactor to use ExponentialBackoff (ironcladlou@gmail.com)
- bump(github.com/openshift/openshift-sdn)
  c08ebda0774795eec624b5ce9063662b19959cf3 (rpenta@redhat.com)
- Auto-generated docs/bash-completions for 'oadm pod-network make-projects-
  global' (rpenta@redhat.com)
- Show empty deployments on overview if latest (spadgett@redhat.com)
- Support retrying mapping creation on conflict (ironcladlou@gmail.com)
- Support installation via containers in new-app (ccoleman@redhat.com)
- Disable create form inputs and submit button while the API request is
  happening (jforrest@redhat.com)
- fix project request with quota (deads@redhat.com)
- UPSTREAM: 16441: Pass runtime.Object to Helper.Create/Replace
  (deads@redhat.com)
- Slim down the extended tests (ccoleman@redhat.com)
- upper case first letter of status message (bparees@redhat.com)
- Rsync fixes (cewong@redhat.com)
- better error message for missing dockerfile (bparees@redhat.com)
- add verbose logging to new-app flake and remove extraneous tryuntil logging
  (bparees@redhat.com)
- Support --binary on new-build (ccoleman@redhat.com)
- Fix text-overflow issue on Safari (sgoodwin@redhat.com)
- bump(github.com/openshift/source-to-image)
  9728b53c11218598acb2cc1b9c8cc762c36f44bc (cewong@redhat.com)
- Include name of port in pod template, switch to port/protocol format
  (jforrest@redhat.com)
- Various logs fixes: (admin@benjaminapetersen.me)
- add --context-dir flag to new-build (bparees@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  22b9a4176435ac4453c30c53799338979ef79050 (rpenta@redhat.com)
- Adding border above builds without a service so that they look more
  connected. (sgoodwin@redhat.com)
- Updates to the log-view so that it's more inline with pod terminal. And
  switch to more subtle ellipsis loader. (sgoodwin@redhat.com)
- Expose all container ports creating from source in the UI
  (spadgett@redhat.com)
- Fail if timeout is reached (ccoleman@redhat.com)
- UPSTREAM: 15975: Validate names in BeforeCreate (jliggitt@redhat.com)
- Bug 1275234 - fixes --resource-version error in scale (ffranz@redhat.com)
- skip project validation when creating a new-project (deads@redhat.com)
- Show route target port in the routes table if its set (jforrest@redhat.com)
- Allow users to click monopod donut charts on overview (spadgett@redhat.com)
- Add verbose output to test execution (marun@redhat.com)
- Show warning popup when builds have a status message (jforrest@redhat.com)
- UPSTREAM: 16241: Deflake wsstream stream_test.go (ccoleman@redhat.com)
- Read kubernetes remote from git repository. (maszulik@redhat.com)
- Fix dind vagrant provisioning and test execution (marun@redhat.com)
- Bug 1261548 - oc run --attach support for DeploymentConfig
  (ffranz@redhat.com)
- Use server time for end time in metrics requests (spadgett@redhat.com)
- Fix typo (jliggitt@redhat.com)
- Update angular-patternfly to version 2.3.3 (spadgett@redhat.com)
- UPSTREAM: 16109: expose attachable pod discovery in factory
  (ffranz@redhat.com)
- libvirt use nfs for dev cluster synced folder (marun@redhat.com)
- report a warning and do not continue on a partial match (bparees@redhat.com)
- Move the cgroup regex to package level. (roque@juniper.net)
- UPSTREAM: 16286: Avoid CPU hotloop on client-closed websocket
  (jliggitt@redhat.com)
- Increase vagrant memory default by 512mb (marun@redhat.com)
- Source dind script from the docker repo (marun@redhat.com)
- dind: Fix disabling of sdn node scheduling (marun@redhat.com)
- Switch dind image to use fedora 21 (marun@redhat.com)
- Only run provision-full.sh for single-vm clusters (marun@redhat.com)
- Fix extended network test invocation (marun@redhat.com)
- Invoke docker with sudo when archiving test logs (marun@redhat.com)
- Enhance dind vagrant deployment (marun@redhat.com)
- Add dind detail to the contribution doc (marun@redhat.com)
- Enhance dind-cluster.sh documentation (marun@redhat.com)
- bump(github.com/openshift/openshift-sdn):
  1e4edc9abb6bb8ac7e5cd946ddec4c10cc714d67 (danw@redhat.com)
- Add rsync daemon copy strategy for windows support (cewong@redhat.com)
- UPSTREAM: 11694: http proxy support for exec/pf (agoldste@redhat.com)
- hack/cherry-pick.sh: fix typo (agoldste@redhat.com)
- hack/cherry-pick.sh: support binary files in diffs (agoldste@redhat.com)
- Added job policy (maszulik@redhat.com)
- platformmanagement_public_514 - Allow importing tags from ImageStreams
  pointing to external registries. (maszulik@redhat.com)
- Minor fix for nfs readme (liangxia@users.noreply.github.com)
- delete: Remove both image stream spec and status tags (mkargaki@redhat.com)
- move newapp commands that need docker into extended tests
  (bparees@redhat.com)
- Only watch pod statuses for overview donut chart (spadgett@redhat.com)
- Vagrantfile should allow multiple sync folders (ccoleman@redhat.com)
- Fix context dir for STI (ccoleman@redhat.com)
- Fixing typos (dmcphers@redhat.com)
- Deflake test/cmd/newapp.sh (ccoleman@redhat.com)
- Review comments (ccoleman@redhat.com)
- Docs (ccoleman@redhat.com)
- Completions (ccoleman@redhat.com)
- Generated conversions (ccoleman@redhat.com)
- Docker and STI builder images support binary extraction (ccoleman@redhat.com)
- Enable binary build endpoint and CLI via start-build (ccoleman@redhat.com)
- UPSTREAM: 15053<carry>: Conversions for v1beta3 (ccoleman@redhat.com)
- UPSTREAM: 15053: Support stdinOnce and fix attach (ccoleman@redhat.com)
- Disable all e2e portforward tests (ccoleman@redhat.com)
- Fixes infinite loop on login and forces auth when password were provided
  (ffranz@redhat.com)
- increase timeout for helper pods (bparees@redhat.com)
- bump(github.com/openshift/source-to-image)
  84e4633329181926ec8d746e189769522b1ff6a7 (roque@juniper.net)
- When running as a pod, pass the network context to source-to-image.
  (roque@juniper.net)
- allow dockersearcher to be setup after config is parsed (bparees@redhat.com)
- UPSTREAM: <carry>: Back n forth downward/metadata conversions
  (maszulik@redhat.com)
- Ensure no overlap between SDN cluster network and service/portal network
  (rpenta@redhat.com)
- [RPMS] expand obsoletes to include OSE versions (sdodson@redhat.com)
- [RPMS] bump docker requirement to 1.8.2 (sdodson@redhat.com)
- UPSTREAM: 15194: Avoid spurious "Hairpin setup failed..." errors
  (danw@gnome.org)
- Change to healthz as per @liggit comments. (smitram@gmail.com)
- Add a monitoring uri to the stats port. This allows us to not affect hosted
  backends but rather use the listener on the stats port to service health
  check requests. Of course the side effect here is that if you turn off stats,
  then the monitoring uri will not be available. (smitram@gmail.com)

* Fri Oct 23 2015 Scott Dodson <sdodson@redhat.com> 3.0.2.903
- Revert "bump(github.com/openshift/openshift-sdn):
  699716b85d1ac5b2f3e48969bbdbbb2a1266e9d0" (sdodson@redhat.com)
- More cleanup of the individual pages, use the extra space better
  (jforrest@redhat.com)
- don't attempt to call create on files that don't exist (bparees@redhat.com)
- Use remove() rather than html('') to empty SVG element (spadgett@redhat.com)
- enable storage interface functions (deads@redhat.com)
- Retry ISM save upon conflicting IS error (maszulik@redhat.com)
- assets: Update topology-graph widget (stefw@redhat.com)
- logs: Re-use from upstream (mkargaki@redhat.com)
- assets: Use $evalAsync when updating topology widget (stefw@redhat.com)
- Handle data gaps when calculating CPU usage in UI (spadgett@redhat.com)
- add SA and role for job and hpa controllers (deads@redhat.com)
- UPSTREAM: 16067: Provide a RetryOnConflict helper for client libraries
  (maszulik@redhat.com)
- UPSTREAM: 10707: logs: Use resource builder (mkargaki@redhat.com)
- Update pod status chart styles (spadgett@redhat.com)
- rework how allow-missing-images resolves (bparees@redhat.com)
- bump(github.com/docker/spdystream): 43bffc4 (agoldste@redhat.com)
- Remove unused godep (agoldste@redhat.com)
- Increase height of metrics utilization sparkline (spadgett@redhat.com)
- Disable a known broken test (ccoleman@redhat.com)
- UID repair should not fail when out of range allocation (ccoleman@redhat.com)
- Add v1beta3 removal notes to UPGRADE.md (ironcladlou@gmail.com)
- Add iptables to origin image (sdodson@redhat.com)
- policy changes for extensions (deads@redhat.com)
- enable extensions (deads@redhat.com)
- UPSTREAM: 7f6f85bd7b47db239868bcd868ae3472373a4f05: fixes attach broken
  during the refactor (ffranz@redhat.com)
- UPSTREAM: 16042: fix missing error handling (deads@redhat.com)
- UPSTREAM: 16084: Use NewFramework in all tests (ccoleman@redhat.com)
- e2e: Verify service account token and log cli errors (ccoleman@redhat.com)
- add commands for managing SCCs (deads@redhat.com)
- Allow patching events (jliggitt@redhat.com)
- Fix problems with overview donut chart on IE (spadgett@redhat.com)
- Bug 1274200: Prompt by default when deleting a non-existent tag
  (mkargaki@redhat.com)
- correct htpasswd error handling: (deads@redhat.com)
- Set build.status.reason on error (rhcarvalho@gmail.com)
- Fixes several issues with tabbed output (ffranz@redhat.com)
- Referenced tags should get updated on a direct tag (ccoleman@redhat.com)
- oc tag should take an explicit reference to ImageStreamImage
  (ccoleman@redhat.com)
- Add back a fix for the libvirt dev_cluster case (danw@redhat.com)
- Completions (ccoleman@redhat.com)
- Fixes 'oc edit' file blocking on windows (ffranz@redhat.com)
- On Windows, use CRLF line endings in oc edit (ccoleman@redhat.com)
- Add iptables requirement to openshift package (sdodson@redhat.com)
- Remove extra copy of main.css from bindata (jforrest@redhat.com)
- add grace period for evacuate (pweil@redhat.com)
- Remove "Pod" prefix from header on browse pod page (spadgett@redhat.com)
- Only try to update build if status message changed (rhcarvalho@gmail.com)
- Show status details for a DC in both the deployments table and DC pages
  (jforrest@redhat.com)
- Bug 1273787 - start deployment btn doesnt get enabled after dep finishes
  (jforrest@redhat.com)
- UPSTREAM: Proxy: do not send X-Forwarded-Host or X-Forwarded-Proto with an
  empty value (cewong@redhat.com)
- handle new non-resource urls (deads@redhat.com)
- UPSTREAM: 15958: add nonResourceURL detection (deads@redhat.com)
- expose: Set route port (mkargaki@redhat.com)
- UPSTREAM: 15461: expose: Enable exposing multiport objects
  (mkargaki@redhat.com)

* Wed Oct 21 2015 Scott Dodson <sdodson@redhat.com> 3.0.2.902
- Revert "Support deleting image stream status tags" (ccoleman@redhat.com)
- bump(github.com/openshift/openshift-sdn):
  699716b85d1ac5b2f3e48969bbdbbb2a1266e9d0 (danw@redhat.com)
- Adjust line-height of route link to avoid clipping (spadgett@redhat.com)
- Support deleting image stream status tags (kargakis@tuta.io)
- tag: Support deleting a tag with -d (mkargaki@redhat.com)
- Bump K8s label selector to next version (sgoodwin@redhat.com)
- Add a container terminal to the pod details page (stefw@redhat.com)
- Remove old labels rendering from individual pages since its duplicate info
  (jforrest@redhat.com)
- Remove duplicate bootstrap.js from dependencies (spadgett@redhat.com)
- updated LDIF and tests (skuznets@redhat.com)
- Bug 1273350 - make click open the secondary nav instead of navigating
  (jforrest@redhat.com)
- Update label key/value pairs truncate at <769. Fixes
  https://github.com/openshift/origin/issues/5181 (sgoodwin@redhat.com)
- remove the deprecated build label on pods (bparees@redhat.com)
- UPSTREAM: 15621: Correctly handle empty source (ccoleman@redhat.com)
- UPSTREAM: 15953: Return unmodified error from negotiate (ccoleman@redhat.com)
- Fix exposing deployment configs (mkargaki@redhat.com)
- Remove code duplication (rhcarvalho@gmail.com)
- Fix patternfly CSS ordering (spadgett@redhat.com)
- Initial impl of viewing logs in web console (admin@benjaminapetersen.me)
- This commit implements birthcry for openshift proxy. This also addresses
  rhbz: https://bugzilla.redhat.com/show_bug.cgi?id=1270474
  (avagarwa@redhat.com)
- Bug 1271989 - error when navigating to resources in diff projects
  (jforrest@redhat.com)
- fix --config flag (deads@redhat.com)
- client side changes for deployment logs (mkargaki@redhat.com)
- server side changes for deployment logs (mkargaki@redhat.com)
- api changes for deployment logs (mkargaki@redhat.com)
- Disable new upstream e2e tests (ccoleman@redhat.com)
- Remove authentication from import (ccoleman@redhat.com)
- Web console scaling (spadgett@redhat.com)
- Bug 1268891 - pods not always grouped when service selector should cover
  template of a dc/deployment (jforrest@redhat.com)
- ImageStream status.dockerImageRepository should always be local
  (ccoleman@redhat.com)
- remove kubectl apply from oc (deads@redhat.com)
- UPSTREAM: <drop>: disable kubectl apply until there's an impl
  (deads@redhat.com)
- Add ng-cloak to navbar to reduce flicker on load (spadgett@redhat.com)
- Disable v1beta3 in REST API (ironcladlou@gmail.com)
- help unit tests compile (maszulik@redhat.com)
- refactors (deads@redhat.com)
- UPSTREAM: openshift-sdn(TODO): update for iptables.New call
  (deads@redhat.com)
- UPSTREAM: openshift-sdn(TODO): handle boring upstream refactors
  (deads@redhat.com)
- UPSTREAM: 12221: Allow custom namespace creation in e2e framework
  (deads@redhat.com)
- UPSTREAM: 15807: Platform-specific setRLimit implementations
  (jliggitt@redhat.com)
- UPSTREAM: TODO: expose ResyncPeriod function (deads@redhat.com)
- UPSTREAM: 15451 <partial>: Add our types to kubectl get error
  (ccoleman@redhat.com)
- UPSTREAM: 14496: deep-copies: Structs cannot be nil (mkargaki@redhat.com)
- UPSTREAM: 11827: allow permissive SA secret ref limitting (deads@redhat.com)
- UPSTREAM: 12498: Re-add timeouts for kubelet which is not in the upstream PR.
  (deads@redhat.com)
- UPSTREAM: 15232: refactor logs to be composeable (deads@redhat.com)
- UPSTREAM: 8890: Allowing ActiveDeadlineSeconds to be updated for a pod
  (deads@redhat.com)
- UPSTREAM: <drop>: tweak generator to handle conversions in other packages
  (deads@redhat.com)
- UPSTREAM: <drop>: make test pass with old codec (deads@redhat.com)
- UPSTREAM: <drop>: add back flag types to reduce noise during this rebase
  (deads@redhat.com)
- UPSTREAM: <none>: Hack date-time format on *util.Time (ccoleman@redhat.com)
- UPSTREAM: <none>: Suppress aggressive output of warning (ccoleman@redhat.com)
- UPSTREAM: <carry>: v1beta3 (deads@redhat.com)
- UPSTREAM: <carry>: support pointing oc exec to old openshift server
  (deads@redhat.com)
- UPSTREAM: <carry>: Back n forth downward/metadata conversions
  (deads@redhat.com)
- UPSTREAM: <carry>: Disable --validate by default (mkargaki@redhat.com)
- UPSTREAM: <carry>: update describer for dockercfg secrets (deads@redhat.com)
- UPSTREAM: <carry>: reallow the ability to post across namespaces in api
  (pweil@redhat.com)
- UPSTREAM: <carry>: helper methods paralleling old latest fields
  (deads@redhat.com)
- UPSTREAM: <carry>: Add deprecated fields to migrate 1.0.0 k8s v1 data
  (jliggitt@redhat.com)
- UPSTREAM: <carry>: SCC (deads@redhat.com)
- UPSTREAM: <carry>: Allow pod start to be delayed in Kubelet
  (ccoleman@redhat.com)
- UPSTREAM: <carry>: Disable UIs for Kubernetes and etcd (deads@redhat.com)
- bump(k8s.io/kubernetes): 4c8e6f47ec23f390978e651232b375f5f9cde3c7
  (deads@redhat.com)
- bump(github.com/coreos/go-etcd): de3514f25635bbfb024fdaf2a8d5f67378492675
  (deads@redhat.com)
- bump(github.com/ghodss/yaml): 73d445a93680fa1a78ae23a5839bad48f32ba1ee
  (deads@redhat.com)
- bump(github.com/fsouza/go-dockerclient):
  1399676f53e6ccf46e0bf00751b21bed329bc60e (deads@redhat.com)
- bump(github.com/prometheus/client_golang):
  3b78d7a77f51ccbc364d4bc170920153022cfd08 (deads@redhat.com)
- Change api version in example apps (jhadvig@redhat.com)
- Bug 1270185 - service link on route details page missing project name
  (jforrest@redhat.com)
- make cherry-pick.sh easier to work with (deads@redhat.com)
- Minor deployment describer formatting fix (ironcladlou@gmail.com)
- Fix deployment config minor ui changes. (sgoodwin@redhat.com)
- Several fixes to the pods page (jforrest@redhat.com)
- test/cmd/export.sh shouldn't dump everything to STDOUT (ccoleman@redhat.com)
- Use the privileged SCC for all kube e2e tests (ccoleman@redhat.com)
- Preserve case of subresources when normalizing URLs (jliggitt@redhat.com)
- Output less info on hack/test-cmd.sh failures (ccoleman@redhat.com)
- Disable potentially insecure TLS cipher suites by default
  (ccoleman@redhat.com)
- Raw sed should not be used in hack/* scripts for Macs (ccoleman@redhat.com)
- Show container metrics in UI (spadgett@redhat.com)
- Fix provisions to be overrides (dmcphers@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  62ec906f6563828364474ef117371ea2ad804dc8 (danw@redhat.com)
- UPSTREAM: Fix non-multitenant pod routing (sdodson@redhat.com)
- Make image trigger test more reliable (ironcladlou@gmail.com)
- [RPMS] atomic-openshift services use openshift bin (sdodson@redhat.com)
- [RPMS] fix rpm build related to sdn restructure (sdodson@redhat.com)
- Fix fuzzing versions in serialization tests (pmorie@gmail.com)
- Show labels on all individual pages. Add label filtering to build config and
  deployment config pages. (jforrest@redhat.com)
- Provide initialized cloud provider in Kubelet config. (jsafrane@redhat.com)
- Fixes to the warning returned by the dc controller (mkargaki@redhat.com)
- Fix govet error (jliggitt@redhat.com)
- adding keystone IdP (sseago@redhat.com)
- Bug 1259260 - when searching docker registry, should not exit in case of no
  matches (ffranz@redhat.com)
- Fix asset config warning (jliggitt@redhat.com)
- Configurable identity mapper strategies (jliggitt@redhat.com)
- Bump proxy resync from 30s to 10m (agoldste@redhat.com)
- Convert secondary nav to a hover menu (sgoodwin@redhat.com)
- remove useless ginkgo test for LDAP (skuznets@redhat.com)
- Sample app readme update (jhadvig@redhat.com)
- Fix s2i build with environment file extended test (mfojtik@redhat.com)
- Return error instead of generating arbitrary names (rhcarvalho@gmail.com)
- Replace local constant with constant from Kube (rhcarvalho@gmail.com)
- Godoc formatting (rhcarvalho@gmail.com)
- Print etcd version when calling openshift version (mfojtik@redhat.com)
- Use cmdutil.PrintSuccess() to display bulk output (ccoleman@redhat.com)
- Add SNI support (jliggitt@redhat.com)
- make ldap sync job accept group/foo whitelists (deads@redhat.com)
- move bash_completion.d/oc to clients package (tdawson@redhat.com)
- Add a cluster diagnostic to check if master is also running as a node.
  (dgoodwin@redhat.com)
- Update the hacking guide (mkargaki@redhat.com)
- add examples for rpm-based installs (jeder@redhat.com)
- Add fibre channel guide (hchen@redhat.com)
- Add Cinder Persistent Volume guide (jsafrane@redhat.com)
- Move NFS documentation to a subchapter. (jsafrane@redhat.com)
- Add environment values to oc new-app help for mysql
  (nakayamakenjiro@gmail.com)
- Update oadm router and registry help message (nakayamakenjiro@gmail.com)
- Enable cpu cfs quota by default (decarr@redhat.com)
- Gluster Docs (screeley@redhat.com)

* Wed Oct 14 2015 Scott Dodson <sdodson@redhat.com> 3.0.2.901
- Build transport for etcd directly (jliggitt@redhat.com)
- Remove volume dir chcon from e2e-docker (agoldste@redhat.com)
- Always try to chcon the volume dir (agoldste@redhat.com)
- Filter service endpoints when using multitenant plugin (danw@redhat.com)
- Update for osdn plugin argument changes (danw@redhat.com)
- Updated generated docs for openshift-sdn changes (danw@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  5a41fee40db41b65578c07eff9fef35d183dce1c (danw@redhat.com)
- Generalize move-upstream.sh (ccoleman@redhat.com)
- Add links to things from the overview and pod template (jforrest@redhat.com)
- deconflict swagger ports (deads@redhat.com)
- Scan for selinux write error in registry diagnostic. (dgoodwin@redhat.com)
- Report transient deployment trigger errors via API field
  (mkargaki@redhat.com)
- Add build timeout, by setting ActiveDeadlineSeconds on a build pod
  (maszulik@redhat.com)
- Update completions (ffranz@redhat.com)
- Append missing flags to cobra flags (jchaloup@redhat.com)
- added an LDAP host label (skuznets@redhat.com)
- add openshift group mapping for ldap sync (deads@redhat.com)
- Convert tables to 2 column layout at mobile res. And fix incorrect url to js
  files. (sgoodwin@redhat.com)
- add ldapsync blacklisting (deads@redhat.com)
- Move to openshift-jvm 1.0.29 (slewis@fusesource.com)
- bump(github.com/openshift/openshift-sdn)
  12f0efeb113058e04e9d333b92bbdddcfc34a9b4 (rpenta@redhat.com)
- Auto generated bash completion and examples doc for oadm pod-network
  (rpenta@redhat.com)
- Remove duplicated helper (rhcarvalho@gmail.com)
- union group name mapper (deads@redhat.com)
- UPSTREAM: 14871: Additional service ports config for master service.
  (abutcher@redhat.com)
- Update completions for kubernetes-service-node-port (abutcher@redhat.com)
- UPSTREAM: 13978 <drop>: NodePort option: Allowing for apiservers behind load-
  balanced endpoint. (abutcher@redhat.com)
- Update image stream page to use a table for the tags (jforrest@redhat.com)
- tighten ldap sync query types (deads@redhat.com)
- refactor building the syncer (deads@redhat.com)
- enhanced active directory ldap sync (deads@redhat.com)
- Use only official Dockerfile parser (rhcarvalho@gmail.com)
- Support oadm pod-network cmd (rpenta@redhat.com)
- added extended tests for LDAP sync (skuznets@redhat.com)
- update ldif to work (deads@redhat.com)
- e2e util has unused import 'regexp' (ccoleman@redhat.com)
- add master API proxy client cert (jliggitt@redhat.com)
- Change CA lifetime defaults (jliggitt@redhat.com)
- UPSTREAM: 15224: Refactor SSH tunneling, fix proxy transport TLS/Dial
  extraction (jliggitt@redhat.com)
- UPSTREAM: 15224: Allow specifying scheme when proxying (jliggitt@redhat.com)
- UPSTREAM: 14889: Honor InsecureSkipVerify flag (jliggitt@redhat.com)
- UPSTREAM: 14967: Add util to set transport defaults (jliggitt@redhat.com)
- Fix for issue where pod template is clipped at mobile res.      - fix
  https://github.com/openshift/origin/issues/4489   - switch to pf-image icon
  - correct icon alignment in pod template        - align label and meta data
  on overview (sgoodwin@redhat.com)
- Fix issue where long text strings extend beyond pod template container at
  mobile res.  - remove flex and min/max width that are no longer needed
  (sgoodwin@redhat.com)
- OS support for host pid and ipc (pweil@redhat.com)
- UPSTREAM:<carry>:hostPid/hostIPC scc support (pweil@redhat.com)
- UPSTREAM:14279:IPC followup (pweil@redhat.com)
- UPSTREAM:<carry>:v1beta3 hostIPC (pweil@redhat.com)
- UPSTREAM:12470:Support containers with host ipc in a pod (pweil@redhat.com)
- UPSTREAM:<carry>:v1beta3 hostPID (pweil@redhat.com)
- UPSTREAM:13447:Allow sharing the host PID namespace (pweil@redhat.com)
- Specify scheme in the jolokia URL (slewis@fusesource.com)
- Fix tag and package name in new-build example (rhcarvalho@gmail.com)
- UPSTREAM: <drop>: disable oidc tests (jliggitt@redhat.com)
- Verify `oc get` returns OpenShift resources (ccoleman@redhat.com)
- UPSTREAM: 15451 <partial>: Add our types to kubectl get error
  (ccoleman@redhat.com)
- Add loading message to all individual pages (jforrest@redhat.com)
- Remove build history and deployment history from main tables
  (jforrest@redhat.com)
- status: Fix incorrect missing registry warning (mkargaki@redhat.com)
- Add configuration options for logging and metrics endpoints
  (spadgett@redhat.com)
- Fix masterCA conversion (jliggitt@redhat.com)
- Fix oc logs (jliggitt@redhat.com)
- Revert "Unique output image stream names in new-app" (ccoleman@redhat.com)
- Bug 1263562 - users without projects should get default ctx when logging in
  (ffranz@redhat.com)
- added LDAP entries for other schemas (skuznets@redhat.com)
- remove cruft from options (deads@redhat.com)
- provide feedback while the ldap group sync job is running (deads@redhat.com)
- Allow POST access to node stats for cluster-reader and system:node-reader
  (jliggitt@redhat.com)
- Update role bindings in compatibility test (jliggitt@redhat.com)
- Add cluster role bindings diagnostic (jliggitt@redhat.com)
- Fix template minification to keep line breaks, remove html files from bindata
  (jforrest@redhat.com)
- Post 4902 fixes (maszulik@redhat.com)
- Add oc rsync command (cewong@redhat.com)
- bump(github.com/openshift/source-to-image)
  1fd4429c584d688d83c1247c03fa2eeb0b083ccb (cewong@redhat.com)
- Fixing typos (dmcphers@redhat.com)
- allocate supplemental groups to namespace (pweil@redhat.com)
- Remove forgotten code no longer used (nagy.martin@gmail.com)
- assets: Fix null dereference in updateTopology() (stefw@redhat.com)
- make oc logs support builds and buildconfigs (deads@redhat.com)
- Wait until the slave pod is gone (nagy.martin@gmail.com)
- UPSTREAM: 14616: Controller framework test flake fix (mfojtik@redhat.com)
- Disable verbose extended run (mfojtik@redhat.com)
- Unique output image stream names in new-app (rhcarvalho@gmail.com)
- Fix start build extended test (mfojtik@redhat.com)
- Make cleanup less noisy (mfojtik@redhat.com)
- Replace default reporter in Ginkgo with SimpleReporter (mfojtik@redhat.com)
- Atomic feature flags followup (miminar@redhat.com)
- examples: Move hello-openshift example to API v1 (stefw@redhat.com)
- Fix Vagrant provisioning after move to contrib/vagrant (dcbw@redhat.com)
- Always bring up openshift's desired network configuration on Vagrant
  provision (dcbw@redhat.com)
- Add annotations to the individual pages (jforrest@redhat.com)
- OS swagger and descriptions (pweil@redhat.com)
- UPSTREAM:<carry>:introduce scc types for fsgroup and supplemental groups
  (pweil@redhat.com)
- ldap sync active directory (deads@redhat.com)
- Change connection-based kubelet auth to application-level authn/authz
  interfaces (jliggitt@redhat.com)
- UPSTREAM: 15232`: refactor logs to be composeable (deads@redhat.com)
- bump(github.com/hashicorp/golang-lru):
  7f9ef20a0256f494e24126014135cf893ab71e9e (jliggitt@redhat.com)
- UPSTREAM: 14700: Add authentication/authorization interfaces to kubelet,
  always include /metrics with /stats (jliggitt@redhat.com)
- UPSTREAM: 14134: sets.String#Intersection (jliggitt@redhat.com)
- UPSTREAM: 15101: Add bearer token support for kubelet client config
  (jliggitt@redhat.com)
- UPSTREAM: 14710: Add verb to authorizer attributes (jliggitt@redhat.com)
- UPSTREAM: 13885: Cherry pick base64 and websocket patches
  (ccoleman@redhat.com)
- Delete --all option for oc export in cli doc (nakayamakenjiro@gmail.com)
- Fix cadvisor in integration test (jliggitt@redhat.com)
- Disable --allow-missing-image test (cewong@redhat.com)
- Make kube-proxy iptables sync period configurable (mkargaki@redhat.com)
- Update to latest version of PatternFly 2.2.0 and Bootstrap  3.3.5
  (sgoodwin@redhat.com)
- PHP hot deploy extended test (jhadvig@redhat.com)
- Ruby hot deploy extended test (jhadvig@redhat.com)
- change show-all default to true (deads@redhat.com)
- Update post-creation messages for builds in CLI (rhcarvalho@gmail.com)
- [Bug 4959] sample-app/cleanup.sh: fix usage of not-installed killall command.
  (vsemushi@redhat.com)
- fix bad oadm line (max.andersen@gmail.com)
- Refactored Openshift Origin builder, decoupled from S2I builder, added mocks
  and testing (kirill.frolov@servian.com)
- Use correct master url for internal token request, set master CA correctly
  (jliggitt@redhat.com)
- fix non-default all-in-one ports for testing (deads@redhat.com)
- Add extended test for git authentication (cewong@redhat.com)
- reconcile-cluster-role-bindings command (jliggitt@redhat.com)
- Change validation timing on create from image page (spadgett@redhat.com)
- Issue 2378 - Show TLS information for routes, create routes and
  routes/routename pages (jforrest@redhat.com)
- Lowercase resource names (rhcarvalho@gmail.com)
- create local images as docker refs (bparees@redhat.com)
- Preserve deployment status sequence (mkargaki@redhat.com)
- Add tests for S2I Perl and Python images with Hot Deploy
  (nagy.martin@gmail.com)
- Test asset config (jliggitt@redhat.com)
- UPSTREAM: 14967: Add util to set transport defaults (jliggitt@redhat.com)
- UPSTREAM: 14246: Fix race in lifecycle admission test (mfojtik@redhat.com)
- Fix send to closed channel in waitForBuild (mfojtik@redhat.com)
- UPSTREAM: 13885: Update error message in wsstream for go 1.5
  (mfojtik@redhat.com)
- Fix go vet (jliggitt@redhat.com)
- Change to use a 503 error page to fully address #4215 This allows custom
  error pages to be layered on in custom haproxy images. (smitram@gmail.com)
- Reduce number of test cases and add cleanup - travis seems to be hitting
  memory errors with starting multiple haproxy routers. (smitram@gmail.com)
- Fixes as per @Miciah review comments. (smitram@gmail.com)
- Update generated completions. (smitram@gmail.com)
- Add/update f5 tests for partition path. (smitram@gmail.com)
- Add partition path support to the f5 router - this will also allows us to
  support sharded routers with f5 using different f5 partitions.
  (smitram@gmail.com)
- Fixes as per @smarterclayton & @pweil- review comments and add generated docs
  and bash completions. (smitram@gmail.com)
- Turn on haproxy statistics by default since its now on a protected page.
  (smitram@gmail.com)
- Bind to router stats options (fixes issue #4884) and add help text
  clarifications. (smitram@gmail.com)
- Add tabs to pod details page (spadgett@redhat.com)
- Bug 1268484 - Use build.metadata.uid to track dismissed builds in UI
  (spadgett@redhat.com)
- cleanup ldap sync validation (deads@redhat.com)
- update auth test to share long running setup (deads@redhat.com)
- make ldap sync-group work (deads@redhat.com)
- fix ldap sync types to be more understandable (deads@redhat.com)
- remove unused mapper (pweil@redhat.com)
- change from kind to resource (pweil@redhat.com)
- fix decoding to handle yaml (deads@redhat.com)
- UPSTREAM: go-ldap: add String for debugging (deads@redhat.com)
- UPSTREAM: 14451: Fix a race in pod backoff. (mfojtik@redhat.com)
- Fix unbound variable in hack/cherry-pick.sh (mfojtik@redhat.com)
- Cleanup godocs of build types (rhcarvalho@gmail.com)
- Update travis to go 1.5.1 (mfojtik@redhat.com)
- Enable Go 1.5 (ccoleman@redhat.com)
- UPSTREAM: Fix typo in e2e pods test (nagy.martin@gmail.com)
- rename allow-missing to allow-missing-images (bparees@redhat.com)
- prune images: Conform to the hacking guide (mkargaki@redhat.com)
- Drop imageStream.spec.dockerImageRepository tags/IDs during conversion
  (ccoleman@redhat.com)
- Add validation to prevent IS.spec.dockerImageRepository from having tags
  (ccoleman@redhat.com)
- Add a helper to move things upstream (ccoleman@redhat.com)
- Cherry-pick helper (ccoleman@redhat.com)
- Clean up root folder (ccoleman@redhat.com)
- UPSTREAM: 13885: Support websockets on exec and pod logs
  (ccoleman@redhat.com)
- add test for patching anonymous fields in structs (deads@redhat.com)
- UPSTREAM: 14985: fix patch for anonymous struct fields (deads@redhat.com)
- fix exec admission controller flake (deads@redhat.com)
- updated template use (skuznets@redhat.com)
- fixed go vet invocation and errors (skuznets@redhat.com)
- Add ethtool to base/Dockerfile.rhel7 too (sdodson@redhat.com)
- [RPMS] atomic-openshift services use openshift bin (sdodson@redhat.com)
- [RPMS] fix rpm build related to sdn restructure (sdodson@redhat.com)
- UPSTREAM: 14831: allow yaml as argument to patch (deads@redhat.com)
- Move host etc master/node directories to official locations.
  (dgoodwin@redhat.com)
- Wait for service account to be accessible (mfojtik@redhat.com)
- Wait for builder account (mfojtik@redhat.com)
- Pull RHEL7 images from internal CI registry (mfojtik@redhat.com)
- Initial addition of S2I SCL enablement extended tests (mfojtik@redhat.com)
- tolerate missing docker images (bparees@redhat.com)
- bump(github.com/spf13/cobra): d732ab3a34e6e9e6b5bdac80707c2b6bad852936
  (ffranz@redhat.com)
- Rename various openshift directories to origin. (dgoodwin@redhat.com)
- allow SAR requests in lifecycle admission (pweil@redhat.com)
- Issue 4001 - add requests and limits to resource limits on settings page
  (jforrest@redhat.com)
- prevent force pull set up from running in other focuses; add some debug clues
  in hack/util.sh; comments from Cesar, Ben; create new builder images so we
  run concurrent;  MIchal's comments; move to just one builder
  (gmontero@redhat.com)
- fix govet example error (skuznets@redhat.com)
- Add Restart=always to master service (sdodson@redhat.com)
- Issue 4867 - route links should open in a new window (jforrest@redhat.com)
- Fix output of git basic credentials in builder (cewong@redhat.com)
- Set build status message in case of error (rhcarvalho@gmail.com)
- Fix the reference of openshift command in Makefile (akira@tagoh.org)
- Issue 4632 - remove 'Project' from the project overview header
  (jforrest@redhat.com)
- Issue 4860 - missing no deployments msg when only have RCs
  (jforrest@redhat.com)
- Support deployment hook volume inheritance (ironcladlou@gmail.com)
- fix RAR test flake (deads@redhat.com)
- UPSTREAM: 14688: Deflake max in flight (deads@redhat.com)
- Fix vagrant provisioning (danw@redhat.com)
- setup makefile to be parallelizeable (deads@redhat.com)
- api group support for authorizer (deads@redhat.com)
- Apply OOMScoreAdjust and Restart policy to openshift node (decarr@redhat.com)
- Fix nit (dmcphers@redhat.com)
- Add ethtool to our deps (mkargaki@redhat.com)
- Issue 4855 - warning flickers about build config and deployment config not
  existing (jforrest@redhat.com)
- Add MySQL extended replication tests (nagy.martin@gmail.com)
- extended: Disable Daemon tests (mkargaki@redhat.com)
- Adds explicit suggestions for some cli commands (ffranz@redhat.com)
- bump(github.com/spf13/pflag): b084184666e02084b8ccb9b704bf0d79c466eb1d
  (ffranz@redhat.com)
- bump(github.com/cpf13/cobra): 046a67325286b5e4d7c95b1d501ea1cd5ba43600
  (ffranz@redhat.com)
- Don't show errors in name field until blurred (spadgett@redhat.com)
- Allow whitespace-only values in UI for required parameters
  (spadgett@redhat.com)
- make RAR allow evaluation errors (deads@redhat.com)
- Watch routes on individual service page (spadgett@redhat.com)
- bump(github.com/vjeantet/ldapserver) 19fbc46ed12348d5122812c8303fb82e49b6c25d
  (mkargaki@redhat.com)
- Bug 1266859: UPSTREAM: <drop>: expose: Truncate service names
  (mkargaki@redhat.com)
- Update docs regarding the rebase (mkargaki@redhat.com)
- add timing statements (deads@redhat.com)
- Routes should be able to specify which port they desire (ccoleman@redhat.com)
- Add dbus for the OVS setup required docker restart. (dgoodwin@redhat.com)
- Return to OSBS preferred labels. (dgoodwin@redhat.com)
- Source info should only be loaded when Git is used for builds
  (ccoleman@redhat.com)
- Rename ose3 node run script to match aos. (dgoodwin@redhat.com)
- Bump aos-master and aos-node to 3.0.2.100. (dgoodwin@redhat.com)
- Rename ose3 script to matching aos, standardize labels. (dgoodwin@redhat.com)
- Merge in latest work from oseonatomic repo. (dgoodwin@redhat.com)
- Expose version as prometheus metric (jimmidyson@gmail.com)
- Cleanup unused code and add proper labels. (dgoodwin@redhat.com)
- Update for AOS 3.1 version and a more consistent bz component name.
  (dgoodwin@redhat.com)
- Add missing Name label. (dgoodwin@redhat.com)
- Cleanup aos-master image for OSBS build. (dgoodwin@redhat.com)
- aos-node container building locally. (dgoodwin@redhat.com)
- Use Dockerfile.product for ose-master to future proof a bit.
  (dgoodwin@redhat.com)
- Move to Dockerfile.product for future upstream compatability.
  (dgoodwin@redhat.com)
- Initial draft of aos-master container. (dgoodwin@redhat.com)

* Tue Sep 29 2015 Scott Dodson <sdodson@redhat.com> 3.0.2.900
- OSE Docs URLs (sdodson@redhat.com)
- Do not treat directories named Dockerfile as file (rhcarvalho@gmail.com)
- Disable the pods per node test - it requires the kubelet stats
  (ccoleman@redhat.com)
- Set Build.Status.Pushspec to resolved pushspec (rhcarvalho@gmail.com)
- Bug: 1266442 1266447 (jhadvig@redhat.com)
- new-app: Better output in case of invalid Dockerfile (mkargaki@redhat.com)
- Compatibility test for Volume Source (mkargaki@redhat.com)
- Update UPGRADE.md about Metadata/Downward (mkargaki@redhat.com)
- get local IP like the server would and add retries to build test watch
  (pweil@redhat.com)
- Interesting refactoring (mkargaki@redhat.com)
- Fix verify-open-ports.sh to not fail on success (mkargaki@redhat.com)
- Boring refactoring; code generations (mkargaki@redhat.com)
- UPSTREAM: openshift-sdn: 167: plugins: Update Kube client imports
  (mkargaki@redhat.com)
- UPSTREAM: <carry>: Move to test pkg to avoid linking test flags in binaries
  (pweil@redhat.com)
- UPSTREAM: <carry>: Add etcd prefix (mkargaki@redhat.com)
- UPSTREAM: 14664: fix testclient prepend (deads@redhat.com)
- UPSTREAM: 14502: don't fatal on missing sorting flag (deads@redhat.com)
- UPSTREAM: <drop>: hack experimental versions and client creation
  (deads@redhat.com)
- UPSTREAM: 14496: deep-copies: Structs cannot be nil (mkargaki@redhat.com)
- UPSTREAM: <carry>: Back n forth downward/metadata conversions
  (mkargaki@redhat.com)
- UPSTREAM: 13728: Allow to replace os.Exit() with panic when CLI command fatal
  (mfojtik@redhat.com)
- UPSTREAM: 14291: add patch verb to APIRequestInfo (deads@redhat.com)
- UPSTREAM: 13910: Fix resourcVersion = 0 in cacher (mkargaki@redhat.com)
- UPSTREAM: 13864: Fix kubelet logs --follow bug (mkargaki@redhat.com)
- UPSTREAM: 14063: enable system CAs (mkargaki@redhat.com)
- UPSTREAM: 13756: expose: Avoid selector resolution if a selector is not
  needed (mkargaki@redhat.com)
- UPSTREAM: 13746: Fix field=metadata.name (ccoleman@redhat.com)
- UPSTREAM: 9870: Allow Volume Plugins to be configurable (deads@redhat.com)
- UPSTREAM: 11827: allow permissive SA secret ref limitting (deads@redhat.com)
- UPSTREAM: 12221: Allow custom namespace creation in e2e framework
  (mfojtik@redhat.com)
- UPSTREAM: 12498: Re-add timeouts for kubelet which is not in the upstream PR.
  (deads@redhat.com)
- UPSTREAM: 9009: Retry service account update when adding token reference
  (deads@redhat.com)
- UPSTREAM: 9844: EmptyDir volume SELinux support (deads@redhat.com)
- UPSTREAM: 7893: scc allocation interface methods (deads@redhat.com)
- UPSTREAM: 7893: scc (pweil@redhat.com)
- UPSTREAM: 8890: Allowing ActiveDeadlineSeconds to be updated for a pod
  (deads@redhat.com)
- UPSTREAM: <drop>: add back flag types to reduce noise during this rebase
  (deads@redhat.com)
- UPSTREAM: <none>: Hack date-time format on *util.Time (ccoleman@redhat.com)
- UPSTREAM: <none>: Suppress aggressive output of warning (ccoleman@redhat.com)
- UPSTREAM: <carry>: Disable --validate by default (mkargaki@redhat.com)
- UPSTREAM: <carry>: update describer for dockercfg secrets (deads@redhat.com)
- UPSTREAM: <carry>: reallow the ability to post across namespaces in api
  (pweil@redhat.com)
- UPSTREAM: <carry>: support pointing oc exec to old openshift server
  (deads@redhat.com)
- UPSTREAM: <carry>: Add deprecated fields to migrate 1.0.0 k8s v1 data
  (jliggitt@redhat.com)
- UPSTREAM: <carry>: Allow pod start to be delayed in Kubelet
  (ccoleman@redhat.com)
- UPSTREAM: <carry>: Disable UIs for Kubernetes and etcd (deads@redhat.com)
- UPSTREAM: <carry>: v1beta3 (deads@redhat.com)
- bump(github.com/emicklei/go-restful) 1f9a0ee00ff93717a275e15b30cf7df356255877
  (mkargaki@redhat.com)
- bump(k8s.io/kubernetes) 86b4e777e1947c1bc00e422306a3ca74cbd54dbe
  (mkargaki@redhat.com)
- Update java console (slewis@fusesource.com)
- fix QueryForEntries API (deads@redhat.com)
- added sync-groups command basics (skuznets@redhat.com)
- add ldap groups sync (skuznets@redhat.com)
- Remove templates.js from bindata and fix HTML minification
  (spadgett@redhat.com)
- fedora 21 Vagrant provisioning fixes (dcbw@redhat.com)
- Issue 4795 - include ref and contextdir in github links (jforrest@redhat.com)
- new-app: Actually use the Docker parser (mkargaki@redhat.com)
- Update old references to _output/local/go/bin (rhcarvalho@gmail.com)
- improved robustness of recycler script (mturansk@redhat.com)
- Do not export test utility (rhcarvalho@gmail.com)
- Change build-go to generate binaries to _output/local/bin/${platform}
  (ccoleman@redhat.com)
- Make deployment trigger logging quieter (ironcladlou@gmail.com)
- bump(github.com/openshift/openshift-sdn)
  669deb4de23ab7f79341a132786b198c7f272082 (rpenta@redhat.com)
- Fix openshift-sdn imports in origin (rpenta@redhat.com)
- Move plugins/osdn to Godeps/workspace/src/github.com/openshift/openshift-
  sdn/plugins/osdn (rpenta@redhat.com)
- Move sdn ovssubnet to pkg/ovssubnet dir (rpenta@redhat.com)
- Reorganize web console create flow (spadgett@redhat.com)
- Fix handle on watchObject. Set deletionTimestamp instead of deleted.
  (jforrest@redhat.com)
- Use direct CLI invocations rather than embedding CLI (ccoleman@redhat.com)
- Next steps page (after creating stuff in console) (ffranz@redhat.com)
- Disable parallelism for now (ccoleman@redhat.com)
- Do not require the master components from test/util (ccoleman@redhat.com)
- [userinterface_public_538] Create individual pages for all resources With
  some changes from @sg00dwin and @spadgett (jforrest@redhat.com)
- Fix some of the govet issues (mfojtik@redhat.com)
- annotate builds on clone (bparees@redhat.com)
- Make network fixup during provision conditional (marun@redhat.com)
- Update roadmap url (dmcphers@redhat.com)
- better error message for immutable edits to builds (bparees@redhat.com)
- Default NetworkConfig.ServiceNetworkCIDR to
  KubernetesMasterConfig.ServicesSubnet (jliggitt@redhat.com)
- create SCCExecRestriction admission plugin (deads@redhat.com)
- added oadm commands to validate node and master config (skuznets@redhat.com)
- Add an e2e test for Dockerfile and review comments (ccoleman@redhat.com)
- Add extended tests for start-build (mfojtik@redhat.com)
- allow local access reviews while namespace is terminating (deads@redhat.com)
- build oc, move to clients, move alt clients to redistributable
  (tdawson@redhat.com)
- Add --kubeconfig support for compat with kubectl (ccoleman@redhat.com)
- assets: Filter topology correctly and refactor relations (stefw@redhat.com)
- allow self SARs using old policy: (deads@redhat.com)
- Capture panic in extended CLI and return them as Go errors
  (mfojtik@redhat.com)
- UPSTREAM: 13728: Allow to replace os.Exit() with panic when CLI command fatal
  (mfojtik@redhat.com)
- Take stdin on OpenShift CLI (ccoleman@redhat.com)
- add extended tests for example repos (bparees@redhat.com)
- added syntax highlighting to readme (skuznets@redhat.com)
- Retry finalizer on conflict error (decarr@redhat.com)
- Build from a Dockerfile directly (ccoleman@redhat.com)
- fix backwards poll args (bparees@redhat.com)
- Add missing rolling hook conversions (ironcladlou@gmail.com)
- Fix potential race conditions during SDN setup (rpenta@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  0f9e6558e8dceb8c8317e3587d9c9c94ae07ecb8 (rpenta@redhat.com)
- Add missing rolling hook conversions (ironcladlou@gmail.com)
- Make "default" an admin namespace in multitenant (danw@redhat.com)
- add patch to default roles (deads@redhat.com)
- Exclude a few more tests (ccoleman@redhat.com)
- Refactor setting and resetting HTTP proxies (rhcarvalho@gmail.com)
- UPSTREAM: 14291: add patch verb to APIRequestInfo (deads@redhat.com)
- Add --portal-net back to all-in-one args (danw@redhat.com)
- Bump kubernetes-ui-label-selector to v0.0.10 - fixes js error
  (jforrest@redhat.com)
- Fix broken link in readme (mtayer@redhat.com)
- add SA role bindings to auto-provisioned namespaces (deads@redhat.com)
- diagnostics: fail gracefully on broken kubeconfig (lmeyer@redhat.com)
- Improve systemd detection for diagnostics. (dgoodwin@redhat.com)
- Refactor pkg/build/builder (rhcarvalho@gmail.com)
- Set kubeconfig in extended tests (ccoleman@redhat.com)
- Extended failure (ccoleman@redhat.com)
- Only run k8s upstream tests that are passing (ccoleman@redhat.com)
- Add example env vars (rhcarvalho@gmail.com)
- Pass env vars defined in Docker build strategy (rhcarvalho@gmail.com)
- [RPMs] Ease the upgrade to v1.0.6 (sdodson@redhat.com)
- BZ1221441 - new filter that shows unique project name (rafabene@gmail.com)
- Push F5 image (ccoleman@redhat.com)
- Making the regex in ./test/cmd/admin.sh a little more flexible for downstream
  (bleanhar@redhat.com)
- Making the regex in ./test/cmd/admin.sh a little more flexible for downstream
  (bleanhar@redhat.com)
- Add generated-by annotation to CLI new-app and web console
  (mfojtik@redhat.com)
- disable go vet in make check-test (skuznets@redhat.com)
- more comments (pweil@redhat.com)
- add cluster roles to diagnostics (deads@redhat.com)
- UPSTREAM: 14063: enable system CAs (deads@redhat.com)
- Linux 386 cross compile (ccoleman@redhat.com)
- Add X-Forwarded-* headers and the new Forwarded header for rfc7239 so that
  the backend has info about the proxied request (and requestor).
  (smitram@gmail.com)
- switch to https for sample repo url (bparees@redhat.com)
- Add SA secret checking to SA readiness test in integration tests
  (cewong@redhat.com)
- Adding source secret (jhadvig@redhat.com)
- disable go vet in make check-test (skuznets@redhat.com)
- Update bash-completion (nakayamakenjiro@gmail.com)
- Allow metadata on images to be edited after creation (ccoleman@redhat.com)
- Enable linux-386 (ccoleman@redhat.com)
- .commit is a file, not a directory (ccoleman@redhat.com)
- Retry deployment resource updates (ironcladlou@gmail.com)
- Improve latest deployment output (ironcladlou@gmail.com)
- Make release extraction a separate step for builds (ccoleman@redhat.com)
- bump(github.com/openshift/openshift-sdn)
  0f9e6558e8dceb8c8317e3587d9c9c94ae07ecb8 (rpenta@redhat.com)
- Fix potential race conditions during SDN setup (rpenta@redhat.com)
- Fix casing of output (ironcladlou@gmail.com)
- Adjust help templates to latest version of Cobra (ffranz@redhat.com)
- Update generated completions (ffranz@redhat.com)
- bump(github.com/spf13/cobra): 6d7031177028ad8c5b4b428ac9a2288fbc1c0649
  (ffranz@redhat.com)
- bump(github.com/spf13/pflag): 8e7dc108ab3a1ab6ce6d922bbaff5657b88e8e49
  (ffranz@redhat.com)
- Update to version 0.0.9 for kubernetes-label-selector. Fixes issue
  https://github.com/openshift/origin/issues/3180 (sgoodwin@redhat.com)
- UPSTREAM: 211: Allow listen only ipv4 (ccoleman@redhat.com)
- UPSTREAM: Disable systemd activation for DNS (ccoleman@redhat.com)
- bump(github.com/skynetservices/skydns):bb2ebadc9746f23e4a296e3cbdb8c01e956bae
  e1 (jimmidyson@gmail.com)
- Fixes #4494: Don't register skydns metrics on nodes (jimmidyson@gmail.com)
- Move positional parameters before package lists (nagy.martin@gmail.com)
- Simplify the readme to point to docs (ccoleman@redhat.com)
- Normalize extended tests into test/extended/*.sh (ccoleman@redhat.com)
- Improve deploy --cancel output (ironcladlou@gmail.com)
- Bump min Docker version in docs (agoldste@redhat.com)
- Normalize extended tests into test/extended/*.sh (ccoleman@redhat.com)
- Return empty Config field in FromName to avoid nil pointer error
  (nakayamakenjiro@gmail.com)
- docs: Fixed broken links in openshift_model.md. (stevem@gnulinux.net)
- Show corrent error message if passed json template is invalid
  (prukner.jan@seznam.cz)
- Clean up test directories (ccoleman@redhat.com)
- hack/build-images.sh fails on vboxfs due to hardlink (ccoleman@redhat.com)
- Fail with stack trace in test bash (ccoleman@redhat.com)
- Make oc rsh behave more like ssh (ccoleman@redhat.com)
- better error messages for parameter errors (bparees@redhat.com)
- Simplify the release output, create a zip (ccoleman@redhat.com)
- Cleaning useless colon (remy.binsztock@tech-angels.com)
- app: Implement initial topology-graph based view (stefw@redhat.com)
- app: Normalize kind property on retrieved items (stefw@redhat.com)
- app: Toggle overview modes between tiles and topology (stefw@redhat.com)
- Test for exposing external services (mkargaki@redhat.com)
- UPSTREAM: 13756: expose: Avoid selector resolution if a selector is not
  needed (mkargaki@redhat.com)
- Add helper script to run kube e2e tests (marun@redhat.com)
- Stop adding user to 'docker' group (marun@redhat.com)

* Fri Sep 18 2015 Scott Dodson <sdodson@redhat.com> 0.2-9
- Rename from openshift -> origin
- Symlink /var/lib/origin to /var/lib/openshift if /var/lib/openshift exists

* Wed Aug 12 2015 Steve Milner <smilner@redhat.com> 0.2-8
- Master configs will be generated if none are found when the master is installed.
- Node configs will be generated if none are found when the master is installed.
- Additional notice file added if config is generated by the RPM.
- All-In-One services removed.

* Wed Aug 12 2015 Steve Milner <smilner@redhat.com> 0.2-7
- Added new ovs script(s) to file lists.

* Wed Aug  5 2015 Steve Milner <smilner@redhat.com> 0.2-6
- Using _unitdir instead of _prefix for unit data

* Fri Jul 31 2015 Steve Milner <smilner@redhat.com> 0.2-5
- Configuration location now /etc/origin
- Default configs created upon installation

* Tue Jul 28 2015 Steve Milner <smilner@redhat.com> 0.2-4
- Added AEP packages

* Mon Jan 26 2015 Scott Dodson <sdodson@redhat.com> 0.2-3
- Update to 21fb40637c4e3507cca1fcab6c4d56b06950a149
- Split packaging of openshift-master and openshift-node

* Mon Jan 19 2015 Scott Dodson <sdodson@redhat.com> 0.2-2
- new package built with tito

* Fri Jan 09 2015 Adam Miller <admiller@redhat.com> - 0.2-2
- Add symlink for osc command line tooling (merged in from jhonce@redhat.com)

* Wed Jan 07 2015 Adam Miller <admiller@redhat.com> - 0.2-1
- Update to latest upstream release
- Restructured some of the golang deps  build setup for restructuring done
  upstream

* Thu Oct 23 2014 Adam Miller <admiller@redhat.com> - 0-0.0.9.git562842e
- Add new patches from jhonce for systemd units

* Mon Oct 20 2014 Adam Miller <admiller@redhat.com> - 0-0.0.8.git562842e
- Update to latest master snapshot

* Wed Oct 15 2014 Adam Miller <admiller@redhat.com> - 0-0.0.7.git7872f0f
- Update to latest master snapshot

* Fri Oct 03 2014 Adam Miller <admiller@redhat.com> - 0-0.0.6.gite4d4ecf
- Update to latest Alpha nightly build tag 20141003

* Wed Oct 01 2014 Adam Miller <admiller@redhat.com> - 0-0.0.5.git6d9f1a9
- Switch to consistent naming, patch by jhonce

* Tue Sep 30 2014 Adam Miller <admiller@redhat.com> - 0-0.0.4.git6d9f1a9
- Add systemd and sysconfig entries from jhonce

* Tue Sep 23 2014 Adam Miller <admiller@redhat.com> - 0-0.0.3.git6d9f1a9
- Update to latest upstream.

* Mon Sep 15 2014 Adam Miller <admiller@redhat.com> - 0-0.0.2.git2647df5
- Update to latest upstream.
