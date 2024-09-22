FROM golang:1.21 AS build
RUN apt update && apt install graphviz-dev -y
WORKDIR /src
COPY . .
RUN go build -o /bin/app ./cmd/main.go 

FROM debian:bookworm-slim

ENV PORT=8080
WORKDIR /bin

COPY --from=build /bin/app ./app
RUN apt update \
    && apt install graphviz curl -y \
    && apt-get clean autoclean \
    && apt-get autoremove --yes \
    && rm -rf /var/lib/{apt,dpkg,cache,log}/

EXPOSE $PORT

HEALTHCHECK --interval=5m --timeout=10s --start-interval=1s --start-period=5s \
    CMD curl -f http://localhost:$PORT/api/v1/healthcheck || exit 1

CMD ["./app"]