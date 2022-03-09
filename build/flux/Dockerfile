FROM ubuntu:latest as base

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

RUN echo 'alias python="/usr/bin/python3.8"' >> /root/.bashrc && \
    echo 'alias pip="/usr/bin/pip3"' >> /root/.bashrc && \
    . /root/.bashrc

RUN echo 'set number' >> /root/.vimrc 

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
    pkg-config && apt -y clean  && apt -y autoremove

RUN mkdir -p /root/flux-install
WORKDIR /root/
RUN git clone https://github.com/flux-framework/flux-core.git 

RUN cd /root/flux-core/ && ./autogen.sh && PYTHON_VERSION=3.8 ./configure --prefix=/root/flux-install \ 
    && make -j $(nproc) && make install && cd /root && rm -rf /root/flux-core

# Install go
WORKDIR /home
RUN wget https://dl.google.com/go/go1.17.7.linux-amd64.tar.gz  && tar -xvf go1.17.7.linux-amd64.tar.gz && \
         mv go /usr/local  &&  rm go1.17.7.linux-amd64.tar.gz

ENV GOROOT "/usr/local/go"
ENV GOPATH "/go"
ENV PATH "$GOROOT/bin:$PATH"
RUN mkdir -p /go/src

WORKDIR /root/
ENV PATH "/root/flux-install/bin:$PATH"
ENV LD_LIBRARY_PATH "/root/flux-install/lib/flux:/root/flux-install/lib"
RUN flux keygen

RUN  git clone https://github.com/cmisale/flux-sched.git --branch api-kubeflux --single-branch && \ 
    cd /root/flux-sched/ \
	&& ./autogen.sh && PYTHON_VERSION=3.8 ./configure --prefix=/root/flux-install && make -j $(nproc) && make install \
	&& cp t/data/resource/jgfs/tiny.json /home \
    && cp t/data/resource/jobspecs/basics/test001.yaml /home \
	&& cp -r resource/hlapi/bindings/c/.libs/* resource/.libs/* /root/flux-install/lib/ \
	&& cp -r resource/hlapi/bindings/go/src/fluxcli /go/src/ \
	&& mv  resource/hlapi/bindings /tmp \
	&& cd /root && rm -rf flux-sched && mkdir -p flux-sched/resource/hlapi && mv /tmp/bindings flux-sched/resource/hlapi

RUN apt purge -y git  python3-dev \
    python3-cffi \
    python3-six \
    python3-yaml \
    python3-jsonschema \
    python3-sphinx \
    python3-pip \
    python3-setuptools \
    && apt -y clean && apt -y autoremove
