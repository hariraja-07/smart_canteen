FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o backend .

FROM alpine:latest
RUN adduser -D -H appuser
COPY --from=build /app/backend /backend
USER appuser
EXPOSE 8080
CMD ["/backend"]