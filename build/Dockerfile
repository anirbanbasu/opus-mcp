# Stage 1: Build the Go application
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Disable CGO to create a statically linked binary suitable for scratch/alpine
RUN CGO_ENABLED=0 GOOS=linux go build -race -a -installsuffix cgo -o opus-mcp .

# Stage 2: Create the final, minimal image
FROM scratch
# Copy the compiled binary from the builder stage
COPY --from=builder /app/opus-mcp /
# Set the command to run the executable
CMD ["/opus-mcp"]