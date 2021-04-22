FROM ubuntu:20.10 as basegoflux

ENV TZ="America/Los_Angeles"
ENV DEBIAN_FRONTEND="noninteractive"
RUN apt-get update #&& yes | unminimize  

RUN apt-get -y upgrade && apt-get -y clean && apt-get -y autoremove

RUN apt-get -y --no-install-recommends install \
    vim \
    less \
    libc6-dev \
    wget \
    systemd \
    git \
    autoconf \
    automake \
    libtool \
    libelf-dev \ 
    libncurses5-dev \
    libssl-dev \
    openssh-client \
    make \
    libsodium-dev \
    libzmq3-dev \
    libczmq-dev \
    uuid-dev \
    libjansson-dev \
    liblz4-dev \
    libhwloc-dev \
    libsqlite3-dev \
    lua5.1 \
    liblua5.1-dev \
    lua-posix \
    python3-dev \
    python3-cffi \
    python3-six \
    python3-yaml \
    python3-jsonschema \
    python3-sphinx \
    python3-pip \
    python3-setuptools \
    aspell \
    aspell-en \
    valgrind \
    libmpich-dev && apt-get -y clean  && apt-get -y autoremove

RUN \
   echo 'alias python="/usr/bin/python3.8"' >> /root/.bashrc && \
   echo 'alias pip="/usr/bin/pip3"' >> /root/.bashrc && \
   . /root/.bashrc

RUN \
   echo 'set number' >> /root/.vimrc 

# Remove python2
RUN apt-get purge -y python2.7-minimal
# You already have Python3 but
# don't care about the version
RUN ln -s /usr/bin/python3 /usr/bin/python
RUN apt-get install -y python3-pip && apt-get -y clean  && apt-get -y autoremove # && ln -s /usr/bin/pip3 /usr/bin/pip
 
#Fluxion
RUN apt-get -y --no-install-recommends install \
    libhwloc-dev \
    libboost-dev \
    libboost-system-dev \
    libboost-filesystem-dev \
    libboost-graph-dev \
    libboost-regex-dev \
    libxml2-dev \
    libyaml-cpp-dev \ 
    python3-yaml \
    pkg-config && apt-get -y clean  && apt-get -y autoremove

RUN cd /root/ && mkdir flux-install
WORKDIR /root/
RUN git clone https://github.com/flux-framework/flux-core.git --branch v0.25.0 --single-branch 
RUN cd /root/flux-core/ && ./autogen.sh && PYTHON_VERSION=3.8 ./configure --prefix=/root/flux-install \ 
    && make && make install && cd /root && rm -rf /root/flux-core
 
RUN cd /root/ && git clone https://github.com/cmisale/flux-sched.git --branch gobind-dev --single-branch
WORKDIR /root/flux-sched/
ENV PATH "/root/flux-install/bin:$PATH"
ENV LD_LIBRARY_PATH "/root/flux-install/lib/flux:/root/flux-install/lib"
RUN flux keygen
RUN ./autogen.sh && PYTHON_VERSION=3.8 ./configure --prefix=/root/flux-install && make -j && make install
# Install go 15
WORKDIR /home
RUN wget https://dl.google.com/go/go1.15.2.linux-amd64.tar.gz  && tar -xvf go1.15.2.linux-amd64.tar.gz && \
         mv go /usr/local  &&  rm go1.15.2.linux-amd64.tar.gz

ENV GOROOT "/usr/local/go"
ENV GOPATH "/go"
ENV PATH "$GOROOT/bin:$PATH"
# WORKDIR /root/flux-sched/
RUN mkdir -p /go/src
RUN cp -r /root/flux-sched/resource/hlapi/bindings/go/src/fluxcli /go/src/ && cd /go/src/fluxcli &&  go mod init
RUN cd /go/src && GOOS=linux CGO_CFLAGS="-I/root/flux-sched/resource/hlapi/bindings/c -I/root/flux-install/include" CGO_LDFLAGS="-L/root/flux-sched/resource/hlapi/bindings/c/.libs -lreapi_cli  -L/root/flux-sched/resource/.libs -lresource -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -L/root/flux-install/lib -lflux-hostlist -lboost_graph -lyaml-cpp" go install -v fluxcli
RUN cp /root/flux-sched/t/scripts/flux-ion-resource.py /root/flux-install/libexec/flux/cmd/


WORKDIR /go/src/sigs.k8s.io/scheduler-plugins
COPY cmd cmd/
COPY hack hack/
COPY pkg  pkg/
COPY scheduler-plugins scheduler-plugins/
COPY test test/
COPY vendor vendor/ 
COPY go.mod .
COPY go.sum .
COPY Makefile .
ARG ARCH
ARG RELEASE_VERSION
ENV BUILDENVVAR CGO_CFLAGS="-I/root/flux-sched/resource/hlapi/bindings/c -I/root/flux-install/include" CGO_LDFLAGS="-L/root/flux-sched/resource/hlapi/bindings/c/.libs -lreapi_cli  -L/root/flux-sched/resource/.libs -lresource -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -L/root/flux-install/lib -lflux-hostlist -lboost_graph -lyaml-cpp"

RUN RELEASE_VERSION=${RELEASE_VERSION} make build-scheduler.$ARCH && mv bin/kube-scheduler /bin/
WORKDIR /bin
RUN mkdir -p /home/data/jgf/
RUN mkdir -p /home/data/jobspecs/

# CMD ["kube-scheduler"]
