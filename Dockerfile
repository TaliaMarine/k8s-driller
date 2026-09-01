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
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/driller"]
