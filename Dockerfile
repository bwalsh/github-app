FROM golang:1.24.13 AS builder
WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /out/github-app ./cmd/github-app

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /out/github-app /github-app

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/github-app"]
