FROM ubuntu:20.04 as builder

ENV DEBIAN_FRONTEND=nonintercative
RUN apt-get update && apt-get install -y build-essential \
    tar \
    autoconf \
    automake \
    make \
    wget \
    git \
    gcc \
    g++ \
    zip \
    openssh-server \
    openssh-client \
    libblas-dev \
    liblapack-dev \
    fftw3-dev fftw3 \
    libxml2 \
    libxml2-dev \
    hdf5-tools \
    libhdf5-dev \
    libboost-all-dev \
    && apt-get clean

#Install OpenMPI
RUN mkdir /tmp/openmpi
WORKDIR /tmp/openmpi
RUN wget https://download.open-mpi.org/release/open-mpi/v4.1/openmpi-4.1.2.tar.gz
RUN tar xfz openmpi-4.1.2.tar.gz \
    && cd openmpi-4.1.2 \
    && ./configure --prefix=/opt/openmpi
RUN cd openmpi-4.1.2 \
    && make all
RUN cd openmpi-4.1.2 \
    && make install

#Add cmake-3.22.2 to resolve FFTW not found issue
RUN mkdir /opt/spack-environment \
    &&  (echo "spack:" \
    &&   echo "  specs:" \
    &&   echo "  - cmake@3.22.2 target=x86_64" \
    &&   echo "  config:" \
    &&   echo "    install_tree: /opt/software" \
    &&   echo "  view: /opt/view") > /opt/spack-environment/spack.yaml

#Again leaving these in case we need to regenerate some input files
#RUN mkdir /coral2
#RUN mkdir /coral2/NiO/
#ADD CORAL2_QMCPACK/NiO/NiO-fcc-supertwist111-supershift000-S8.h5 /coral2/NiO/
#RUN ls /coral2/NiO

#Adding multiple packages through spack runs into a bug that 
#Todd Gamblin has encountered as well. So, adding it the rest here.
#Build cmake and also, qmcpack that does not have deprecated MPI calls
RUN cd /opt/spack-environment \
    && git clone https://github.com/spack/spack.git \
    && . ./spack/share/spack/setup-env.sh \
    && spack env activate . \
    && spack clean -m \
    && spack install --reuse --fail-fast \
    && spack gc -y \
    && cd /root \
    && wget https://github.com/QMCPACK/qmcpack/archive/refs/tags/v3.13.0.tar.gz \
    && tar -zxvf v3.13.0.tar.gz \
    && cd qmcpack-3.13.0/build  \
    && cmake -DCMAKE_C_COMPILER=mpicc -DCMAKE_CXX_COMPILER=mpicxx -DENABLE_SOA=1 -DQMC_MIXED_PRECISION=1 .. && make;
#This is for reference in case we need to run the ctest that helps in generating the run config for strong scaling experiment
#    && cmake -DCMAKE_C_COMPILER=mpicc -DCMAKE_CXX_COMPILER=mpicxx -DENABLE_SOA=1 -DQMC_MIXED_PRECISION=1 -DQMC_DATA=/coral2 .. && make;

# Now we build the QMCPack image
FROM ubuntu:20.04

RUN apt-get update
RUN DEBIAN_FRONTEND="noninteractive" apt-get -y install --no-install-recommends \
    libnuma-dev \
    libgomp1 \
    libevent-core-2.1-7 \
    libevent-pthreads-2.1-7 \
    hwloc \
    fftw3 \
    hdf5-tools \
    liblapack-dev \
    libblas-dev \
    libibverbs1 \
    openssh-server \
    openssh-client \
    pdsh \
    dnsutils \
    && apt-get clean \
    && apt-get autoremove \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

RUN mkdir -p /opt/openmpi
COPY --from=builder /opt/openmpi /opt/openmpi/

# Add priviledge separation directoy to run sshd as root.
RUN mkdir -p /var/run/sshd

# Allow OpenSSH to talk to containers without asking for confirmation
# by disabling StrictHostKeyChecking.
# mpi-operator mounts the .ssh folder from a Secret. For that to work, we need
# to disable UserKnownHostsFile to avoid write permissions.
# Disabling StrictModes avoids directory and files read permission checks.
ARG port=22
RUN sed -i "s/[ #]\(.*StrictHostKeyChecking \).*/ \1no/g" /etc/ssh/ssh_config \
    && echo "    UserKnownHostsFile /dev/null" >> /etc/ssh/ssh_config \
    && sed -i "s/[ #]\(.*Port \).*/ \1$port/g" /etc/ssh/ssh_config \
    && sed -i "s/#\(StrictModes \).*/\1no/g" /etc/ssh/sshd_config \
    && sed -i "s/#\(Port \).*/\1$port/g" /etc/ssh/sshd_config

RUN mkdir -p /root/qmcpack
COPY --from=builder /root/qmcpack-3.13.0/build/bin/qmcpack /root/qmcpack/
ADD NiO /coral2/NiO     
RUN ls /coral2/NiO
WORKDIR /coral2/NiO/dmc-a32-e384-batched_driver-DU32/
#RUN mpirun --allow-run-as-root -n 4 --bind-to none /root/qmcpack/qmcpack NiO-fcc-S8-dmc-strongscale.xml
