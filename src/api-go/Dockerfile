# build stage
FROM golang:1.25-alpine AS build

WORKDIR /app

COPY api-go/go.mod api-go/go.sum ./

RUN go mod download

COPY api-go/ .

RUN CGO_ENABLED=0 go build -o /out/api-go ./main.go

# Runtime stage
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

COPY --from=build /out/api-go /app/api-go
COPY common/playbooks /app/playbooks

ENV ANSIBLE_HOST_KEY_CHECKING=false

EXPOSE 8080

ENTRYPOINT ["/app/api-go"]
