FROM quay.io/eduardoarango/flux:latest

RUN apt install -y protobuf-compiler curl && \
 go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26 && \ 
 go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1

ENV PATH "$PATH:/go/bin"
RUN go get google.golang.org/protobuf/reflect/protoreflect@v1.26.0 && \
go get google.golang.org/protobuf/runtime/protoimpl@v1.26.0

COPY . /go/src/kubeflux/
WORKDIR /go/src/kubeflux/

RUN make server
RUN mkdir -p /home/data/jobspecs && mkdir -p /home/data/jgf
