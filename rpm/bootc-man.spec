Name:           bootc-man
Version:        0.1.0
Release:        1%{?dist}
Summary:        CLI tool for bootable container image testing and verification

License:        Apache-2.0
URL:            https://github.com/tnk4on/bootc-man
Source0:        %{url}/archive/v%{version}/%{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.22
BuildRequires:  git-core
BuildRequires:  make

# Mandatory runtime dependencies
Requires:       podman
Requires:       gvisor-tap-vsock

# VM dependencies (similar to podman-machine subpackage)
%if %{defined fedora}
%ifarch x86_64
Requires:       qemu-system-x86-core
%endif
%ifarch aarch64
Requires:       qemu-system-aarch64-core
%endif
%else
Requires:       qemu-kvm
%endif
Requires:       qemu-img

# Recommended (not strictly required)
Recommends:     openssh-clients
Recommends:     edk2-ovmf

%description
bootc-man (bootc manager) is a CLI tool for building, testing, and
verifying bootable container images. It provides local OCI registry
management, CI pipeline automation, and VM-based boot testing.

%prep
%autosetup -n %{name}-%{version}

%build
make build VERSION=%{version}

%install
install -Dpm 755 bin/%{name} %{buildroot}%{_bindir}/%{name}

# Shell completions (same directory layout as Podman)
install -d %{buildroot}%{_datadir}/bash-completion/completions
install -d %{buildroot}%{_datadir}/zsh/site-functions
install -d %{buildroot}%{_datadir}/fish/vendor_completions.d
./bin/%{name} completion bash > %{buildroot}%{_datadir}/bash-completion/completions/%{name}
./bin/%{name} completion zsh  > %{buildroot}%{_datadir}/zsh/site-functions/_%{name}
./bin/%{name} completion fish > %{buildroot}%{_datadir}/fish/vendor_completions.d/%{name}.fish

%check

%files
%license LICENSE
%doc README.md
%{_bindir}/%{name}
%{_datadir}/bash-completion/completions/%{name}
# Own zsh/fish dirs to avoid requiring those packages
%dir %{_datadir}/zsh/site-functions
%{_datadir}/zsh/site-functions/_%{name}
%dir %{_datadir}/fish/vendor_completions.d
%{_datadir}/fish/vendor_completions.d/%{name}.fish

%changelog
%autochangelog
