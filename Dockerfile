FROM golang:1.26.1 AS builder
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/github-app ./cmd/github-app

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/github-app /github-app

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/github-app"]
