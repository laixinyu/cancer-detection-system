FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /out/backend ./cmd/server
RUN go build -o /out/detection-service ./cmd/detection-service
RUN go build -o /out/report-service ./cmd/report-service
RUN go build -o /out/governance-service ./cmd/governance-service

FROM alpine:3.20
WORKDIR /app
RUN adduser -D appuser
COPY --from=build /out/backend /app/backend
COPY --from=build /out/detection-service /app/detection-service
COPY --from=build /out/report-service /app/report-service
COPY --from=build /out/governance-service /app/governance-service
USER appuser
EXPOSE 8080
CMD ["/app/backend"]
