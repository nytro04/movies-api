# Stage 1: Build the Go application
FROM golang:1.22-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy source code
COPY . .

# Download and install dependencies
RUN go mod download && go mod verify && go mod tidy && go mod vendor

# Build the Go app
RUN GOOS=linux GOARCH=amd64 go build -o api ./cmd/api

# Stage 2: Create a minimal image with the built executable
FROM scratch

# Set the working directory inside the container
WORKDIR /app

# Copy the built executable from the builder stage
COPY --from=builder /app/api .

# Expose port 4000 to the outside world
EXPOSE 4000

# Command to run the executable
CMD ["./api"]