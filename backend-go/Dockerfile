FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /out/backend ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN adduser -D appuser
COPY --from=build /out/backend /app/backend
USER appuser
EXPOSE 8080
CMD ["/app/backend"]
