FROM golang:1.17 AS builder

ARG VERSION

# Setup the environment
ENV CGO_ENABLED=0
ENV GOPATH=/go
ENV GO111MODULE=on
ENV GOBIN=/usr/local/bin/ 
WORKDIR /workspace

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN go build -o /workspace/bin/test-app .


FROM alpine:3.15
COPY --from=builder /workspace/bin/test-app /bin/test-app
ENTRYPOINT /bin/test-app