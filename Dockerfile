FROM golang:1.19.3 AS builder
COPY entrypoint /build
WORKDIR /build
RUN go build -o entrypoint


FROM centos:7.9.2009

LABEL 'com.github.actions.name'='Build RPM'
LABEL 'com.github.actions.description'='Build RPM package from spec'

RUN yum -y install \
    epel-release-7 \
    centos-release-scl-2 \
    createrepo-0.9.9 \
    rpm-build-4.11.3 \
    rpmdevtools-8.3 \
    yum-plugin-priorities-1.1.31 \
    && yum -y groupinstall 'Development Tools' \
    && yum -y clean all

COPY --from=builder /build/entrypoint /entrypoint
ENTRYPOINT [ "/entrypoint" ]

# find-debuginfo.sh doesn't like paths shorther than /usr/src/debug
WORKDIR /usr/src/rpmbuild
