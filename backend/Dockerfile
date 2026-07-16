# Multi-target build: one Go module, three binaries. docker-compose selects the
# entrypoint with `target: api | bot | worker`.

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api    ./cmd/api    \
 && CGO_ENABLED=0 go build -trimpath -o /out/bot    ./cmd/bot    \
 && CGO_ENABLED=0 go build -trimpath -o /out/worker ./cmd/worker

FROM alpine:3.20 AS runtime
RUN adduser -D -u 10001 app
USER app

FROM runtime AS api
COPY --from=build /out/api /usr/local/bin/app
EXPOSE 8080
ENTRYPOINT ["app"]

FROM runtime AS bot
COPY --from=build /out/bot /usr/local/bin/app
EXPOSE 8081
ENTRYPOINT ["app"]

FROM runtime AS worker
COPY --from=build /out/worker /usr/local/bin/app
EXPOSE 8082
ENTRYPOINT ["app"]
