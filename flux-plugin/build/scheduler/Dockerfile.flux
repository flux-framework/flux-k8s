FROM ubuntu:latest as basegoflux

RUN apt -y update && apt -y upgrade && apt -y clean && apt -y autoremove

RUN DEBIAN_FRONTEND=noninteractive apt install -y --no-install-recommends tzdata && apt -y --no-install-recommends install \
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
    && make -j && make install && cd /home && rm -rf /home/flux-core

# Install go 16
WORKDIR /home
RUN wget https://dl.google.com/go/go1.16.10.linux-amd64.tar.gz  && tar -xvf go1.16.10.linux-amd64.tar.gz && \
         mv go /usr/local  &&  rm go1.16.10.linux-amd64.tar.gz

ENV GOROOT "/usr/local/go"
ENV GOPATH "/go"
ENV PATH "$GOROOT/bin:$PATH"
RUN mkdir -p /go/src

ENV PATH "/home/flux-install/bin:$PATH"
ENV LD_LIBRARY_PATH "/home/flux-install/lib/flux:/home/flux-install/lib"
RUN flux keygen

RUN  git clone https://github.com/cmisale/flux-sched.git --branch api-kubeflux --single-branch && \ 
    cd /home/flux-sched/ \
	&& ./autogen.sh && PYTHON_VERSION=3.8 ./configure --prefix=/home/flux-install && make -j && make install \
	&& cp t/data/resource/jgfs/tiny.json /home \
    && cp t/data/resource/jobspecs/basics/test001.yaml /home \
	&& cp -r resource/hlapi/bindings/c/.libs/* resource/.libs/* /home/flux-install/lib/ \
	&& cp -r resource/hlapi/bindings/go/src/fluxcli /go/src/ \
	&& mv  resource/hlapi/bindings /tmp \
	&& cd /home && rm -rf flux-sched && mkdir -p flux-sched/resource/hlapi && mv /tmp/bindings flux-sched/resource/hlapi

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

WORKDIR /go/src 
RUN cd fluxcli &&  go mod init fluxcli && go mod tidy 
# &&  GOOS=linux CGO_CFLAGS="-I/root/flux-sched/resource/hlapi/bindings/c -I/root/flux-install/include" CGO_LDFLAGS="-L/root/flux-install/lib -lreapi_cli  -L/root/flux-install/lib -lresource -L/root/flux-install/lib -lflux-idset -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -L/root/flux-install/lib -lflux-hostlist -lboost_graph -lyaml-cpp" go install fluxcli 

RUN apt install -y protobuf-compiler curl && \
 go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26 && \ 
 go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1

ENV PATH "$PATH:/go/bin"
RUN go get google.golang.org/protobuf/reflect/protoreflect@v1.26.0 && \
go get google.golang.org/protobuf/runtime/protoimpl@v1.26.0

COPY kubeflux /go/src/kubeflux/
COPY Makefile /go/src/kubeflux/
WORKDIR /go/src/kubeflux/
# RUN  protoc --go_out=. --go_opt=paths=source_relative \
#     --go-grpc_out=. --go-grpc_opt=paths=source_relative \
#     fluxcli-grpc/fluxcli.proto

RUN go mod tidy
RUN make server
RUN mkdir -p /home/data/jobspecs && mkdir -p /home/data/jgf
RUN chmod -R ugo+rwx /home/data
# COPY examples/data/kubecluster.json /home/data/jgf/kubecluster.json
# BELOW IS NEEDED ONLY TO BUILD THE SCHEDULER PLUGIN

# WORKDIR /go/src 
# RUN cd fluxcli &&  go mod init fluxcli && go mod tidy &&  GOOS=linux CGO_CFLAGS="-I/root/flux-sched/resource/hlapi/bindings/c -I/root/flux-install/include" CGO_LDFLAGS="-L/root/flux-install/lib/ -lreapi_cli  -L/root/flux-install/lib/ -lresource -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -L/root/flux-install/lib -lflux-hostlist -lboost_graph -lyaml-cpp" go install fluxcli 


# WORKDIR /go/src/sigs.k8s.io/scheduler-plugins
# COPY cmd cmd/
# COPY hack hack/
# COPY pkg  pkg/
# COPY flux-k8s/flux-plugin/kubeflux pkg/kubeflux/
# COPY test test/
# COPY go.mod .
# COPY go.sum .
# COPY Makefile .
# ARG ARCH
# ARG RELEASE_VERSION

# RUN RELEASE_VERSION=${RELEASE_VERSION} make build-scheduler.$ARCH 

# RUN mkdir -p /home/data/jgf/ && mv /home/tiny.json /home/data/jgf/



# FROM ubuntu

# # Copy Flux libraries and headers
# COPY --from=basegoflux /root/flux-install /root/flux-install/
# COPY --from=basegoflux /root/flux-sched/resource/hlapi /root/flux-sched/resource/hlapi/
# COPY --from=basegoflux /root/flux-install/lib/libflux-hostlist* /usr/local/lib/
# COPY --from=basegoflux /go/src/fluxcli  /go/src/fluxcli/
# COPY --from=basegoflux /go/src/sigs.k8s.io/scheduler-plugins/bin/kube-scheduler /bin/kube-scheduler
# COPY --from=basegoflux /home/data/jgf/ /home/data/jgf/

# # Reinstall dependencies we need
# #Fluxion
# RUN  apt -y upgrade && apt -u update && apt -y clean && apt-get -y autoremove

# RUN DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends tzdata && apt -y --no-install-recommends install \
#     libhwloc-dev \
#     libboost-dev \
#     libboost-system-dev \
#     libboost-filesystem-dev \
#     libboost-graph-dev \
#     libboost-regex-dev \
#     libxml2-dev \
#     libyaml-cpp-dev \ 
#     libsodium-dev \
#     libzmq3-dev \
#     libczmq-dev \
#     uuid-dev \
#     libjansson-dev \
#     liblz4-dev \
#     libhwloc-dev \
#     libsqlite3-dev \
#     wget \
#     make \
#     libc6-dev  \
#     lua5.1 liblua5.1-dev lua-posix && apt-get -y clean  && apt-get -y autoremove
# RUN mkdir -p /home/data/jobspecs/
# COPY flux-k8s/flux-plugin/manifests/kubeflux/sched-config.yaml /home/sched-config.yaml
# WORKDIR /bin
# CMD ["kube-scheduler"]
