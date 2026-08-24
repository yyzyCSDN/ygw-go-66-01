FROM golang:1.23.12 AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o /auditlog ./cmd/auditlog

FROM golang:1.23.12
ENV GOPROXY=off GOSUMDB=off
WORKDIR /app
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .
COPY --from=build /auditlog /usr/local/bin/auditlog
EXPOSE 8090
CMD ["/usr/local/bin/auditlog", "-addr", "0.0.0.0:8090", "-dir", "/app/data"]
