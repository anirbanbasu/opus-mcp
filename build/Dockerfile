# Stage 1: Build the Go application
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BUILD_VERSION="v.0.0.1-container"
# Disable CGO to create a statically linked binary suitable for scratch/alpine
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X 'opus-mcp/internal/metadata.BuildVersion=${BUILD_VERSION}' -X 'opus-mcp/internal/metadata.BuildTime=$(date)'" -a -installsuffix cgo -o opus-mcp .

# Stage 2: Create the final, minimal image
FROM scratch
# Copy the compiled binary from the builder stage
COPY --from=builder /app/opus-mcp /opus-mcp
EXPOSE 8000
# Set the command to run the executable
CMD [ "/opus-mcp", "-transport", "http", "-host", "0.0.0.0", "-port", "8000" ]