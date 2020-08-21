Name:		perfcollector
Version:	0.1.0
Release:	1%{?dist}
Summary:	Performance statistics collector and processor.

License:	Proprietary software
URL:		https://github.com/businessperformancetuning/perfcollector

BuildRequires:	git

%define NVdir   %{name}-%{version}

%description
Performance statistics collector and processor.

%prep
rm -rf %{NVdir}
git clone %{url}.git %{NVdir}


%build
cd %{NVdir}
go build ./cmd/perfcollectord
go build ./cmd/perfprocessord

%install
install -D %{_builddir}/%{NVdir}/perfcollectord %{buildroot}%{_bindir}/perfcollectord
install -D %{_builddir}/%{NVdir}/perfprocessord %{buildroot}%{_bindir}/perfprocessord

%post
%systemd_post perfcollectord.service

%preun
%systemd_preun perfcollectord.service

%postun
%systemd_postun_with_restart perfcollectord.service

%files
/usr/bin/perfcollectord
/usr/bin/perfprocessord

%doc



%changelog

