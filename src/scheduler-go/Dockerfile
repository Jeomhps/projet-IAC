# Build stage (auto-detect host arch)
FROM golang:1.22-alpine AS build
WORKDIR /app

COPY scheduler-go/go.mod ./
COPY scheduler-go/go.sum ./
RUN go mod download

COPY scheduler-go/ .

RUN CGO_ENABLED=0 go build -o /out/scheduler-go ./main.go

FROM alpine:3.20
RUN apk add --no-cache \
    python3 py3-pip \
    ansible \
    openssh-client \
    sshpass \
    openssl \
    ca-certificates \
    tzdata

WORKDIR /app

# runtime stage
COPY --from=build /out/scheduler-go /app/scheduler-go

COPY common/playbooks /app/playbooks

ENV ANSIBLE_HOST_KEY_CHECKING=false
ENTRYPOINT ["/app/scheduler-go"]
