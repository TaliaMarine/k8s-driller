# syntax=docker/dockerfile:1

FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /driller ./cmd/driller

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=backend /driller ./driller
COPY --from=frontend /src/frontend/dist ./frontend/dist
# Numeric, not the name "nonroot": distroless has no /etc/passwd, so naming
# the user here (even though it matches the base image's own default)
# overwrites the image config's User field with an unresolvable string.
# Kubernetes' kubelet then refuses to start the pod under
# securityContext.runAsNonRoot (it can't verify a non-numeric user is
# actually non-root) with "cannot verify user is non-root". 65532:65532 is
# the UID:GID distroless's own nonroot user already resolves to.
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/driller"]
