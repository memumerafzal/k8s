# ---- build stage ----------------------------------------------------------
FROM golang:1.23-alpine AS build
WORKDIR /src

# No external deps, but copy go.mod first so this layer caches independently.
COPY go.mod ./
COPY main.go ./

ARG VERSION=dev
# Static, stripped binary — CGO off so it runs on distroless/scratch.
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/hello-k8s .

# ---- runtime stage --------------------------------------------------------
# distroless static:nonroot — no shell, no package manager, runs as uid 65532.
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /out/hello-k8s /hello-k8s
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/hello-k8s"]
