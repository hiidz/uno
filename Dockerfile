# The node and go stages run on the build machine's own platform and
# cross-compile, so a multi-platform build emulates nothing but the final
# stage's file copies.

# --- frontend build ---
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-build
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- go build ---
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-build /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /uno ./cmd/uno
# distroless has no shell to mkdir/chown at runtime, so the volume mount
# point is prepared here and copied across with the right owner — Docker
# copies a named volume's initial ownership from the image path it's
# mounted over, the first time that (empty) volume is populated.
RUN mkdir -p /data && chown 65532:65532 /data

# --- final ---
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=go-build /uno ./uno
COPY --from=go-build --chown=nonroot:nonroot /data /data
USER nonroot:nonroot
EXPOSE 8123
ENTRYPOINT ["/app/uno"]
