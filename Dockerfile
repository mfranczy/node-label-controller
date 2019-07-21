FROM golang:1.12 AS builder

ARG project=/nlc

RUN mkdir ${project}
COPY . ${project}
WORKDIR ${project}
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o node-label-controller -mod vendor *.go

FROM alpine:latest

WORKDIR /root/
COPY --from=builder /nlc/node-label-controller .
ENTRYPOINT [ "./node-label-controller" ]