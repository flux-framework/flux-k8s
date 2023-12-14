FROM ubuntu:latest as base

RUN apt -y update && apt -y upgrade && apt -y clean && apt -y autoremove
ENV DEBIAN_FRONTEND=noninteractive
ENV GO_VERSION=1.19.10
ENV INSTALL_PREFIX=/usr

RUN apt install -y --no-install-recommends tzdata && \
    apt -y --no-install-recommends install \
    aspell \
    aspell-en \
    autoconf \
    automake \
    curl \
    git \    
    libc6-dev \
    libczmq-dev \
    libmpich-dev \
    libncurses5-dev \
    libelf-dev \ 
    libssl-dev \
    libtool \
    libsodium-dev \
    libzmq3-dev \
    libjansson-dev \
    liblz4-dev \
    libhwloc-dev \
    libsqlite3-dev \
    lua5.1 \
    liblua5.1-dev \
    lua-posix \
    make \
    openssh-client \
    python3-dev \
    python3-cffi \
    python3-six \
    python3-yaml \
    python3-jsonschema \
    python3-sphinx \
    python3-pip \
    python3-setuptools \
    systemd \
    wget \
    uuid-dev && \
    apt -y clean  && apt -y autoremove

RUN echo 'alias python="/usr/bin/python3.8"' >> /root/.bashrc && \
    echo 'alias pip="/usr/bin/pip3"' >> /root/.bashrc && \
    . /root/.bashrc

RUN echo 'set number' >> /root/.vimrc 

# Install cmake for new build system
RUN curl -s -L https://github.com/Kitware/CMake/releases/download/v3.26.4/cmake-3.26.4-linux-$(uname -m).sh > cmake.sh ;\
    bash cmake.sh --prefix=/usr/local --skip-license ;\
    rm cmake.sh

# Remove Python 2
RUN apt purge -y python2.7-minimal

# Python 3 should be linked to python
RUN ln -s /usr/bin/python3 /usr/bin/python
RUN apt install -y python3-pip \
    &&  apt -y --no-install-recommends install \
    libhwloc-dev \
    libboost-dev \
    libboost-system-dev \
    libboost-filesystem-dev \
    libboost-graph-dev \
    libboost-regex-dev \
    libxml2-dev \
    libyaml-cpp-dev \ 
    python3-yaml \
    libedit-dev \
    libarchive-dev \
    pkg-config && apt -y clean  && apt -y autoremove

RUN git clone https://github.com/flux-framework/flux-core.git /home/flux-core && \
    cd /home/flux-core/ && \
    ./autogen.sh && \
    PYTHON_VERSION=3 ./configure --prefix=${INSTALL_PREFIX} && \ 
    make && make install && \
    cd ../ && \
    rm -rf flux-core

# Install go 19.10
RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz  && tar -xvf go${GO_VERSION}.linux-amd64.tar.gz && \
    mv go /usr/local && rm go${GO_VERSION}.linux-amd64.tar.gz

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH="$GOROOT/bin:$PATH"
RUN mkdir -p /go/src
RUN flux keygen

ENV WITH_GO=yes
RUN git clone https://github.com/flux-framework/flux-sched.git /home/flux-sched && \ 
    cd /home/flux-sched/ && \
    # Ensure we pin to variant that has STATIC - will update when fix is in
    git fetch && git checkout v0.31.0 && \
    # These need to be shared libraries 
    # https://github.com/flux-framework/flux-sched/pull/1094
    sed -i 's/add_library(resource STATIC/add_library(resource SHARED/g' resource/CMakeLists.txt && \
    sed -i 's/add_library ( reapi_module STATIC/add_library ( reapi_module SHARED/g' resource/reapi/bindings/CMakeLists.txt && \
    sed -i 's/add_library ( reapi_cli STATIC/add_library ( reapi_cli SHARED/g' resource/reapi/bindings/CMakeLists.txt && \
    sed -i 's/add_library ( jobspec_conv STATIC/add_library ( jobspec_conv SHARED/g' resource/libjobspec/CMakeLists.txt && \
    PYTHON_VERSION=3 ./configure --prefix=${INSTALL_PREFIX} && \
    make && make install

RUN apt purge -y \
    python3-dev \
    python3-cffi \
    python3-six \
    python3-yaml \
    python3-jsonschema \
    python3-sphinx \
    python3-pip \
    python3-setuptools \
    && apt -y clean && apt -y autoremove

ENV PATH=/usr/local/go/bin:$PATH
RUN apt -y update && apt -y upgrade && apt install --no-install-recommends -y protobuf-compiler curl && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26 && \ 
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1

# These need to be on the LD_LIBRARY_PATH for the server to find at runtime
# This mimcs what we use to build server
ENV LD_LIBRARY_PATH="${LD_LIBRARY_PATH}:/usr/lib:/home/flux-sched/resource:/home/flux-sched/resource/libjobspec:/home/flux-sched/resource/reapi/bindings"
COPY fluence Makefile /go/src/fluence/
WORKDIR /go/src/fluence/

# This is the 0.31.0 tag of flux-sched (same as we install above)
RUN go get -u github.com/flux-framework/flux-sched/resource/reapi/bindings/go/src/fluxcli@250eac78a6753253fc8353a3504d7e843d1b6b24 && \
    go mod tidy && \
    make server FLUX_SCHED_ROOT=/home/flux-sched INSTALL_PREFIX=${INSTALL_PREFIX} && \
    mkdir -p /home/data/jobspecs /home/data/jgf && \
    chmod -R ugo+rwx /home/data