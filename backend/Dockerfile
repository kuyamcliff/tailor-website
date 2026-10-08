# syntax=docker/dockerfile:1
# Build context: repository root (the seed reads the 3D asset manifest from the frontend).
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /app/api
COPY frontend/public/3d/manifest.json /app/asset-manifest.json
ENV APP_ENV=production \
    HTTP_ADDR=:8080 \
    ASSET_MANIFEST_PATH=/app/asset-manifest.json
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
CMD ["serve"]
