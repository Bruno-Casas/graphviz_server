FROM golang:1.21 as build
RUN apt update && apt install graphviz-dev -y
WORKDIR /src
COPY . .
RUN go build -o /bin/app ./cmd/main.go 

FROM debian:bookworm-slim
WORKDIR /bin
COPY --from=build /bin/app ./app
RUN apt update \
    && apt install graphviz -y \
    && apt-get clean autoclean \
    && apt-get autoremove --yes \
    && rm -rf /var/lib/{apt,dpkg,cache,log}/
CMD ["./app"]