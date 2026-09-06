# pogu single-binary image: the admin UI is built with Node, the Go
# binary embeds it, and distroless runs it. No shell or package
# manager in the final image; first-run setup is handled by the
# binary itself (see README "Docker").
FROM node:22-bookworm-slim AS uibuild
WORKDIR /src
COPY ui/package.json ui/package-lock.json ./ui/
RUN cd ui && npm ci
COPY ui ./ui
RUN mkdir -p internal/web/dist && cd ui && npm run build

FROM golang:1.27-bookworm AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=uibuild /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /pogu ./cmd/pogu
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=gobuild /pogu /pogu
COPY --from=gobuild --chown=65532:65532 /out/data /data
ENV POGU_DATA_DIR=/data
EXPOSE 8099
VOLUME /data
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
  CMD ["/pogu", "status", "--data-dir", "/data"]
ENTRYPOINT ["/pogu"]
CMD ["serve", "--listen", "0.0.0.0:8099"]
