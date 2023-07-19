FROM ubuntu:latest as basegoflux

RUN apt -y update && apt -y upgrade && apt -y clean && apt -y autoremove

RUN DEBIAN_FRONTEND=noninteractive apt install -y --no-install-recommends tzdata && apt -y --no-install-recommends install \
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
    libmpich-dev && apt -y clean  && apt -y autoremove

RUN \
   echo 'alias python="/usr/bin/python3.8"' >> /root/.bashrc && \
   echo 'alias pip="/usr/bin/pip3"' >> /root/.bashrc && \
   . /root/.bashrc

RUN \
   echo 'set number' >> /root/.vimrc 

# Remove python2
RUN apt purge -y python2.7-minimal
# You already have Python3 but
# don't care about the version
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
RUN python --version
RUN cd /home/ && mkdir flux-install
WORKDIR /home/
RUN git clone https://github.com/flux-framework/flux-core.git 

RUN cd /home/flux-core/ && ./autogen.sh && PYTHON_VERSION=3 ./configure --prefix=/home/flux-install \ 
    && make && make install && cd /home && rm -rf /home/flux-core

# Install go 19
WORKDIR /home
RUN wget https://go.dev/dl/go1.19.10.linux-amd64.tar.gz  && tar -xvf go1.19.10.linux-amd64.tar.gz && \
         mv go /usr/local && rm go1.19.10.linux-amd64.tar.gz

ENV GOROOT "/usr/local/go"
ENV GOPATH "/go"
ENV PATH "$GOROOT/bin:$PATH"
RUN mkdir -p /go/src

ENV PATH "/home/flux-install/bin:$PATH"
ENV LD_LIBRARY_PATH "/home/flux-install/lib/flux:/home/flux-install/lib"
RUN flux keygen

RUN git clone -b updates-to-golang-pr https://github.com/researchapps/flux-sched.git --single-branch && \ 
    cd /home/flux-sched/ \
	&& ./autogen.sh && PYTHON_VERSION=3.8 ./configure --prefix=/home/flux-install && make && make install
RUN	cd /home/flux-sched && \
    cp -r resource/reapi/bindings/c/.libs/* resource/.libs/* /home/flux-install/lib/ \
	&& cp -r resource/reapi/bindings/go/src/fluxcli /go/src/ \
	&& mv  resource/reapi/bindings /tmp \
	&& cd /home && mkdir -p flux-sched/resource/reapi && mv /tmp/bindings flux-sched/resource/reapi

RUN apt purge -y git  python3-dev \
    python3-cffi \
    python3-six \
    python3-yaml \
    python3-jsonschema \
    python3-sphinx \
    python3-pip \
    python3-setuptools \
    && apt -y clean && apt -y autoremove

# BUILD SHARED LIBRARY
# RUN cp -r /go/src/fluxcli /usr/local/go/src/
# WORKDIR /usr/local/go/src/fluxcli

# RUN CGO_CFLAGS="-I/root/flux-sched/resource/hlapi/bindings/c -I/root/flux-install/include" CGO_LDFLAGS="-L/root/flux-install/lib/ -lreapi_cli  -L/root/flux-install/lib/ -lresource -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -L/root/flux-install/lib -lflux-hostlist -lboost_graph -lyaml-cpp" go install -buildmode=shared -linkshared fluxcli
ENV PATH=/usr/local/go/bin:$PATH
RUN apt -y update && apt -y upgrade && apt install --no-install-recommends -y protobuf-compiler curl && \
   go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26 && \ 
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1

COPY fluence /go/src/fluence/
COPY Makefile /go/src/fluence/
WORKDIR /go/src/fluence/
RUN go mod tidy
RUN make server
RUN mkdir -p /home/data/jobspecs && mkdir -p /home/data/jgf
RUN chmod -R ugo+rwx /home/data
