FROM golang:1.16 AS builder
COPY entrypoint /build
WORKDIR /build
RUN go build -o entrypoint


FROM docker.io/library/centos:7

LABEL 'com.github.actions.name'='Build RPM'
LABEL 'com.github.actions.description'='Build RPM package from spec'

RUN yum -y install \
    epel-release-7 \
    createrepo-0.9.9 \
    rpm-build-4.11.3 \
    rpmdevtools-8.3 \
    && yum -y groupinstall 'Development Tools' \
    && yum -y clean all

COPY --from=builder /build/entrypoint /entrypoint
ENTRYPOINT [ "/entrypoint" ]

# find-debuginfo.sh doesn't like paths shorther than /usr/src/debug
WORKDIR /usr/src/rpmbuild
